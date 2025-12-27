export interface ValidationRules {
	required?: boolean;
	minLength?: number;
	maxLength?: number;
	email?: boolean;
	pattern?: RegExp;
}

export function validateField(value: string, rules: ValidationRules): boolean {
	if (rules.required && (!value || value.trim().length === 0)) {
		return false;
	}

	if (rules.minLength && value.length < rules.minLength) {
		return false;
	}

	if (rules.maxLength && value.length > rules.maxLength) {
		return false;
	}

	if (rules.email) {
		const emailRegex = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;
		if (!emailRegex.test(value)) {
			return false;
		}
	}

	if (rules.pattern && !rules.pattern.test(value)) {
		return false;
	}

	return true;
}
