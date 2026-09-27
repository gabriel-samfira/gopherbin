// Copyright 2019 Gabriel-Adrian Samfira
//
//    Licensed under the Apache License, Version 2.0 (the "License"); you may
//    not use this file except in compliance with the License. You may obtain
//    a copy of the License at
//
//         http://www.apache.org/licenses/LICENSE-2.0
//
//    Unless required by applicable law or agreed to in writing, software
//    distributed under the License is distributed on an "AS IS" BASIS, WITHOUT
//    WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied. See the
//    License for the specific language governing permissions and limitations
//    under the License.

package sql

import (
	"context"
	"regexp"
	"strings"

	gErrors "gopherbin/errors"
	"gopherbin/models"
	"gopherbin/params"

	"github.com/pkg/errors"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var labelNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9 ._-]{0,31}$`)

// normalizeLabel case-folds and validates a label name. Names are unique
// per scope (owner or team) after normalization.
func normalizeLabel(name string) (string, error) {
	norm := strings.ToLower(strings.Join(strings.Fields(name), " "))
	if !labelNameRe.MatchString(norm) {
		return "", gErrors.NewBadRequestError("invalid label %q: use 1-32 letters, digits, space, dot, dash or underscore", name)
	}
	return norm, nil
}

// dedupeLabels normalizes a list of label names, preserving order and
// removing duplicates. A maximum of 16 labels is enforced.
func dedupeLabels(names []string) ([]string, error) {
	out := make([]string, 0, len(names))
	seen := map[string]bool{}
	for _, name := range names {
		norm, err := normalizeLabel(name)
		if err != nil {
			return nil, err
		}
		if seen[norm] {
			continue
		}
		seen[norm] = true
		out = append(out, norm)
	}
	if len(out) > 16 {
		return nil, gErrors.NewBadRequestError("a paste may have at most 16 labels")
	}
	return out, nil
}

// getOrCreateLabel resolves a label by name within its scope, creating it
// on first use. Exactly one of ownerID / teamID must be non-zero.
func getOrCreateLabel(tx *gorm.DB, name string, ownerID uint, teamID uint) (models.Label, error) {
	var label models.Label
	q := tx.Where("name = ?", name)
	if ownerID != 0 {
		q = q.Where("owner_user_id = ? AND team_id IS NULL", ownerID)
	} else {
		q = q.Where("team_id = ? AND owner_user_id IS NULL", teamID)
	}
	if err := q.First(&label).Error; err == nil {
		return label, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Label{}, errors.Wrap(err, "looking up label")
	}

	label = models.Label{Name: name}
	if ownerID != 0 {
		oid := ownerID
		label.OwnerUserID = &oid
	}
	if teamID != 0 {
		tid := teamID
		label.TeamID = &tid
	}
	if err := tx.Create(&label).Error; err != nil {
		return models.Label{}, errors.Wrap(err, "creating label")
	}
	return label, nil
}

// resolveOrCreateLabels maps label names to label rows within the given
// scope, creating any that do not exist yet.
func resolveOrCreateLabels(tx *gorm.DB, names []string, ownerID uint, teamID uint) ([]models.Label, error) {
	labels := make([]models.Label, 0, len(names))
	for _, name := range names {
		label, err := getOrCreateLabel(tx, name, ownerID, teamID)
		if err != nil {
			return nil, err
		}
		labels = append(labels, label)
	}
	return labels, nil
}

// resolveExistingLabels maps label names to label rows within the given
// scope without creating anything. Names that do not exist are skipped.
func resolveExistingLabels(tx *gorm.DB, names []string, ownerID uint, teamID uint) []models.Label {
	if len(names) == 0 {
		return nil
	}
	q := tx.Where("name IN ?", names)
	if ownerID != 0 {
		q = q.Where("owner_user_id = ? AND team_id IS NULL", ownerID)
	} else {
		q = q.Where("team_id = ? AND owner_user_id IS NULL", teamID)
	}
	var labels []models.Label
	if err := q.Find(&labels).Error; err != nil {
		return nil
	}
	return labels
}

// canManageLabel reports whether the user may rename/recolor/delete a label:
// personal labels belong to their owner; team labels to the team owner or any
// active member except viewers (the vocabulary is collectively curated, and
// viewers are read-only).
func canManageLabel(tx *gorm.DB, label *models.Label, userID uint) (bool, error) {
	if label.TeamID == nil {
		return label.OwnerUserID != nil && *label.OwnerUserID == userID, nil
	}
	var team models.Teams
	if err := tx.First(&team, *label.TeamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, errors.Wrap(err, "fetching label team")
	}
	if team.OwnerID == userID {
		return true, nil
	}
	var cnt int64
	if err := tx.Model(&models.TeamUser{}).
		Where("teams_id = ? AND users_id = ? AND status = ? AND (role IS NULL OR role <> ?)",
			team.ID, userID, models.TeamMembershipActive, models.RoleViewer).
		Count(&cnt).Error; err != nil {
		return false, errors.Wrap(err, "counting team membership")
	}
	return cnt > 0, nil
}

// labelScope restricts a label query to the scope (personal owner or team)
// of the given label.
func labelScope(q *gorm.DB, label models.Label) *gorm.DB {
	if label.TeamID != nil {
		return q.Where("team_id = ? AND owner_user_id IS NULL", *label.TeamID)
	}
	if label.OwnerUserID != nil {
		return q.Where("owner_user_id = ? AND team_id IS NULL", *label.OwnerUserID)
	}
	return q.Where("owner_user_id IS NULL AND team_id IS NULL")
}

func labelUsage(tx *gorm.DB, labelID uint) (int64, error) {
	var cnt int64
	if err := tx.Table("paste_labels").Where("label_id = ?", labelID).Count(&cnt).Error; err != nil {
		return 0, errors.Wrap(err, "counting label usage")
	}
	return cnt, nil
}

// ListOwnedLabels returns the viewer's personal labels with usage counts,
// for the label-management section of the settings page.
func (p *paste) ListOwnedLabels(ctx context.Context) ([]params.LabelInfo, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "fetching user")
	}
	type row struct {
		ID         uint
		Name       string
		Color      string
		UsageCount int64
	}
	var rows []row
	// "usage" is a reserved word in MySQL, hence the longer alias.
	if err := p.conn.Model(&models.Label{}).
		Select("labels.id, labels.name, labels.color, COUNT(pl.paste_id) AS usage_count").
		Joins("LEFT JOIN paste_labels pl ON pl.label_id = labels.id").
		Where("labels.owner_user_id = ? AND labels.team_id IS NULL", user.ID).
		Group("labels.id").
		Order("labels.name ASC").
		Scan(&rows).Error; err != nil {
		return nil, errors.Wrap(err, "listing owned labels")
	}
	out := make([]params.LabelInfo, 0, len(rows))
	for _, r := range rows {
		out = append(out, params.LabelInfo{ID: r.ID, Name: r.Name, Color: r.Color, Usage: r.UsageCount})
	}
	return out, nil
}

// UpdateLabel renames and/or recolors a single label the caller manages. A
// rename onto an existing label of the same scope merges the two.
func (p *paste) UpdateLabel(ctx context.Context, labelID uint, args params.UpdateLabelParams) (params.LabelInfo, error) {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return params.LabelInfo{}, errors.Wrap(err, "fetching user")
	}
	var newName string
	if args.Name != nil {
		newName, err = normalizeLabel(*args.Name)
		if err != nil {
			return params.LabelInfo{}, err
		}
	}
	var out params.LabelInfo
	err = p.conn.Transaction(func(tx *gorm.DB) error {
		var label models.Label
		if err := tx.First(&label, labelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(err, "fetching label")
		}
		ok, err := canManageLabel(tx, &label, user.ID)
		if err != nil {
			return err
		}
		if !ok {
			// The label row was loaded successfully: a 401 here would let
			// any logged-in user enumerate label IDs. Deny with 404 instead.
			return gErrors.ErrNotFound
		}
		if newName != "" && newName != label.Name {
			var existing models.Label
			err := labelScope(tx.Where("name = ?", newName), label).First(&existing).Error
			if err == nil {
				// merge into the existing label of the same scope
				var pasteIDs []uint
				if err := tx.Table("paste_labels").
					Where("label_id = ?", label.ID).
					Pluck("paste_id", &pasteIDs).Error; err != nil {
					return errors.Wrap(err, "loading label usage")
				}
				for _, pid := range pasteIDs {
					link := struct {
						PasteID uint `gorm:"column:paste_id"`
						LabelID uint `gorm:"column:label_id"`
					}{PasteID: pid, LabelID: existing.ID}
					if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
						Table("paste_labels").Create(&link).Error; err != nil {
						return errors.Wrap(err, "merging label usage")
					}
				}
				if err := tx.Exec("DELETE FROM paste_labels WHERE label_id = ?", label.ID).Error; err != nil {
					return errors.Wrap(err, "clearing merged label usage")
				}
				if err := tx.Delete(&label).Error; err != nil {
					return errors.Wrap(err, "deleting merged label")
				}
				label = existing
			} else if errors.Is(err, gorm.ErrRecordNotFound) {
				if err := tx.Model(&label).Update("name", newName).Error; err != nil {
					return errors.Wrap(err, "renaming label")
				}
				label.Name = newName
			} else {
				return errors.Wrap(err, "looking up rename target")
			}
		}
		// Applied after a possible merge so the color lands on the label
		// that survives it.
		if args.Color != nil {
			if err := tx.Model(&models.Label{}).Where("id = ?", label.ID).Update("color", *args.Color).Error; err != nil {
				return errors.Wrap(err, "updating label color")
			}
			label.Color = *args.Color
		}
		usage, err := labelUsage(tx, label.ID)
		if err != nil {
			return err
		}
		out = params.LabelInfo{ID: label.ID, Name: label.Name, Color: label.Color, Usage: usage}
		return nil
	})
	if err != nil {
		return params.LabelInfo{}, err
	}
	return out, nil
}

// DeleteLabel removes a label the caller manages from every paste that
// carries it and from its scope's vocabulary.
func (p *paste) DeleteLabel(ctx context.Context, labelID uint) error {
	user, err := p.getUserFromContext(ctx)
	if err != nil {
		return errors.Wrap(err, "fetching user")
	}
	return p.conn.Transaction(func(tx *gorm.DB) error {
		var label models.Label
		if err := tx.First(&label, labelID).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return gErrors.ErrNotFound
			}
			return errors.Wrap(err, "fetching label")
		}
		ok, err := canManageLabel(tx, &label, user.ID)
		if err != nil {
			return err
		}
		if !ok {
			// Post-load denial: 404, so foreign label IDs are not
			// distinguishable from nonexistent ones.
			return gErrors.ErrNotFound
		}
		if err := tx.Exec("DELETE FROM paste_labels WHERE label_id = ?", labelID).Error; err != nil {
			return errors.Wrap(err, "clearing label usage")
		}
		if err := tx.Delete(&models.Label{}, labelID).Error; err != nil {
			return errors.Wrap(err, "deleting label")
		}
		return nil
	})
}
