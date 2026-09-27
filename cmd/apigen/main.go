// apigen enriches the generated swagger.yaml with JSON field schemas derived
// from the Go structs the API actually marshals, so TypeScript codegen (which
// cannot see through x-go-type aliases) produces usable types. It also turns
// the x-public operation extension into an empty security requirement, which
// go-swagger route annotations cannot express.
package main

import (
	"fmt"
	"os"
	"reflect"
	"strings"

	"gopkg.in/yaml.v3"

	"gopherbin/apiserver/responses"
	"gopherbin/params"
)

// aliasMap maps each x-go-type alias in swagger-models.yaml to the Go type
// it names. An alias missing here fails the generation.
func aliasMap() map[string]reflect.Type {
	return map[string]reflect.Type{
		"APIErrorResponse":        reflect.TypeOf(responses.APIErrorResponse{}),
		"JWTResponse":             reflect.TypeOf(params.JWTResponse{}),
		"LabelInfo":               reflect.TypeOf(params.LabelInfo{}),
		"LabelVocabulary":         reflect.TypeOf(params.LabelVocabulary{}),
		"MeSettingsParams":        reflect.TypeOf(params.MeSettingsParams{}),
		"NewTeamParams":           reflect.TypeOf(params.NewTeamParams{}),
		"NewPasteParams":          reflect.TypeOf(params.NewPasteParams{}),
		"NewUserParams":           reflect.TypeOf(params.NewUserParams{}),
		"PasswordLoginParams":     reflect.TypeOf(params.PasswordLoginParams{}),
		"Paste":                   reflect.TypeOf(params.Paste{}),
		"PasteLabelsParams":       reflect.TypeOf(params.PasteLabelsParams{}),
		"PasteListResult":         reflect.TypeOf(params.PasteListResult{}),
		"PasteShareListResponse":  reflect.TypeOf(params.PasteShareListResponse{}),
		"PasteLabel":              reflect.TypeOf(params.PasteLabel{}),
		"SetTeamMemberRoleParams": reflect.TypeOf(params.SetTeamMemberRoleParams{}),
		"TeamInviteInfo":          reflect.TypeOf(params.TeamInviteInfo{}),
		"TeamLabelGroup":          reflect.TypeOf(params.TeamLabelGroup{}),
		"TeamLabelsParams":        reflect.TypeOf(params.TeamLabelsParams{}),
		"TeamListResult":          reflect.TypeOf(params.TeamListResult{}),
		"TeamMember":              reflect.TypeOf(params.TeamMember{}),
		"TeamMemberParams":        reflect.TypeOf(params.TeamMemberParams{}),
		"TeamStats":               reflect.TypeOf(params.TeamStats{}),
		"TeamTransferInfo":        reflect.TypeOf(params.TeamTransferInfo{}),
		"TeamTransferParams":      reflect.TypeOf(params.TeamTransferParams{}),
		"Teams":                   reflect.TypeOf(params.Teams{}),
		"UpdateLabelParams":       reflect.TypeOf(params.UpdateLabelParams{}),
		"UpdatePasteParams":       reflect.TypeOf(params.UpdatePasteParams{}),
		"UpdateTeamParams":        reflect.TypeOf(params.UpdateTeamParams{}),
		"UpdateUserPayload":       reflect.TypeOf(params.UpdateUserPayload{}),
		"UserActionRequest":       reflect.TypeOf(params.UserActionRequest{}),
		"UserListResult":          reflect.TypeOf(params.UserListResult{}),
		"UserSearchResult":        reflect.TypeOf(params.UserSearchResult{}),
		"Users":                   reflect.TypeOf(params.Users{}),
	}
}

func main() {
	path := os.Args[1]
	raw, err := os.ReadFile(path)
	if err != nil {
		panic(err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(raw, &root); err != nil {
		panic(err)
	}
	defs := mapValue(root.Content[0], "definitions")
	if defs == nil || defs.Kind != yaml.MappingNode {
		panic("no definitions block")
	}
	markPublicOperations(mapValue(root.Content[0], "paths"))
	aliases := aliasMap()
	unknown := 0
	for i := 0; i+1 < len(defs.Content); i += 2 {
		name := defs.Content[i].Value
		def := defs.Content[i+1]
		if def.Kind != yaml.MappingNode {
			continue
		}
		xf := mapValue(def, "x-go-type")
		if xf == nil {
			continue
		}
		goType := mapValue(xf, "type")
		if goType == nil {
			continue
		}
		t, known := aliases[goType.Value]
		if !known {
			fmt.Fprintf(os.Stderr, "apigen: definition %q aliases unknown type %q; add it to aliasMap\n", name, goType.Value)
			unknown++
			continue
		}
		schema := structSchemaNode(t)
		var kept []*yaml.Node
		for j := 0; j+1 < len(def.Content); j += 2 {
			if def.Content[j].Value == "x-go-type" {
				continue
			}
			kept = append(kept, def.Content[j], def.Content[j+1])
		}
		// Keep only keys the generated alias did not already define.
		existing := map[string]bool{}
		for j := 0; j+1 < len(kept); j += 2 {
			existing[kept[j].Value] = true
		}
		for j := 0; j+1 < len(schema.Content); j += 2 {
			if existing[schema.Content[j].Value] {
				continue
			}
			kept = append(kept, schema.Content[j], schema.Content[j+1])
		}
		def.Content = kept
	}
	if unknown > 0 {
		os.Exit(1)
	}
	out, err := yaml.Marshal(&root)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		panic(err)
	}
}

// markPublicOperations replaces the x-public: true extension of an
// operation with `security: []`, exempting it from the global Bearer
// requirement.
func markPublicOperations(paths *yaml.Node) {
	if paths == nil || paths.Kind != yaml.MappingNode {
		return
	}
	for i := 1; i < len(paths.Content); i += 2 {
		ops := paths.Content[i]
		if ops.Kind != yaml.MappingNode {
			continue
		}
		for j := 1; j < len(ops.Content); j += 2 {
			op := ops.Content[j]
			if op.Kind != yaml.MappingNode {
				continue
			}
			for k := 0; k+1 < len(op.Content); k += 2 {
				if op.Content[k].Value != "x-public" {
					continue
				}
				if op.Content[k+1].Value == "true" {
					op.Content[k].Value = "security"
					op.Content[k+1] = &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq", Style: yaml.FlowStyle}
				} else {
					op.Content = append(op.Content[:k], op.Content[k+2:]...)
				}
				break
			}
		}
	}
}

func mapValue(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

// structSchemaNode renders the Go struct as a Swagger object schema node,
// keeping field order stable (declaration order) for reproducible specs.
func structSchemaNode(t reflect.Type) *yaml.Node {
	type prop struct {
		name  string
		node  *yaml.Node
		valid bool
	}
	var props []prop
	required := []string{}
	seen := map[string]bool{}
	var walk func(reflect.Type)
	walk = func(t reflect.Type) {
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if f.Anonymous {
				ft := f.Type
				if ft.Kind() == reflect.Ptr {
					ft = ft.Elem()
				}
				if ft.Kind() == reflect.Struct && f.Tag.Get("json") == "" {
					walk(ft)
					continue
				}
			}
			if !f.IsExported() {
				continue // encoding/json ignores unexported fields
			}
			tag := f.Tag.Get("json")
			if tag == "-" {
				continue
			}
			name := strings.Split(tag, ",")[0]
			if name == "" {
				name = f.Name
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			n, ok := swaggerTypeNode(f.Type)
			props = append(props, prop{name: name, node: n, valid: ok})
			if ok && !strings.Contains(tag, "omitempty") {
				required = append(required, name)
			}
		}
	}
	walk(t)

	propMap := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for _, p := range props {
		if !p.valid {
			continue
		}
		propMap.Content = append(propMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: p.name}, p.node)
	}
	schema := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "object"},
		{Kind: yaml.ScalarNode, Tag: "!!str", Value: "properties"},
		propMap,
	}}
	if len(required) > 0 {
		reqSeq := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, r := range required {
			reqSeq.Content = append(reqSeq.Content, &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: r})
		}
		schema.Content = append(schema.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "required"}, reqSeq)
	}
	return schema
}

func swaggerTypeNode(t reflect.Type) (*yaml.Node, bool) {
	switch t.Kind() {
	case reflect.String:
		return scalarMap("type", "string"), true
	case reflect.Bool:
		return scalarMap("type", "boolean"), true
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return scalarMap("type", "integer"), true
	case reflect.Float32, reflect.Float64:
		return scalarMap("type", "number"), true
	case reflect.Ptr:
		return swaggerTypeNode(t.Elem())
	case reflect.Slice, reflect.Array:
		if t.Elem().Kind() == reflect.Uint8 {
			return scalarMap("type", "string", "format", "byte"), true
		}
		inner, ok := swaggerTypeNode(t.Elem())
		if !ok {
			return nil, false
		}
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "array"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "items"},
			inner,
		}}, true
	case reflect.Map:
		if t.Elem().Kind() != reflect.String {
			return nil, false
		}
		return &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map", Content: []*yaml.Node{
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "object"},
			{Kind: yaml.ScalarNode, Tag: "!!str", Value: "additionalProperties"},
			scalarMap("type", "string"),
		}}, true
	case reflect.Struct:
		if t.PkgPath() == "time" && t.Name() == "Time" {
			return scalarMap("type", "string", "format", "date-time"), true
		}
		return structSchemaNode(t), true
	}
	return nil, false
}

func scalarMap(kv ...string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	for i := 0; i+1 < len(kv); i += 2 {
		n.Content = append(n.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[i]},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: kv[i+1]})
	}
	return n
}
