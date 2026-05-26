// Package validator registers custom validation tags with both:
//   - a standalone *validator.Validate (exported as V) for service-layer use
//   - Gin's request-binding engine, so DTO `binding:"strong_password,e164"`
//     tags actually run when c.ShouldBindJSON is called.
//
// RegisterWithGin must be called once during process startup (e.g. from main).
package validator

import (
	"regexp"
	"unicode"

	"github.com/gin-gonic/gin/binding"
	"github.com/go-playground/validator/v10"
	"github.com/wearwhere/wearwhere_be/internal/shared/slug"
)

var V = func() *validator.Validate {
	v := validator.New()
	_ = v.RegisterValidation("strong_password", strongPassword)
	_ = v.RegisterValidation("e164", e164)
	_ = v.RegisterValidation("slug", slugValidator)
	return v
}()

// RegisterWithGin attaches our custom tags to Gin's underlying validator.
// Safe to call multiple times; subsequent calls overwrite the same tags.
func RegisterWithGin() {
	if v, ok := binding.Validator.Engine().(*validator.Validate); ok {
		_ = v.RegisterValidation("strong_password", strongPassword)
		_ = v.RegisterValidation("e164", e164)
		_ = v.RegisterValidation("slug", slugValidator)
	}
}

// strongPassword: min 8 chars, at least 1 number AND 1 special char (per SRS UC08)
func strongPassword(fl validator.FieldLevel) bool {
	s := fl.Field().String()
	if len(s) < 8 {
		return false
	}
	var hasNumber, hasSpecial bool
	for _, r := range s {
		switch {
		case unicode.IsDigit(r):
			hasNumber = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSpecial = true
		}
	}
	return hasNumber && hasSpecial
}

var e164Re = regexp.MustCompile(`^\+[1-9]\d{7,14}$`)

func e164(fl validator.FieldLevel) bool {
	return e164Re.MatchString(fl.Field().String())
}

func slugValidator(fl validator.FieldLevel) bool {
	return slug.IsValid(fl.Field().String())
}
