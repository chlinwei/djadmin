package middleware

import (
	"strings"

	"autoadmin/internal/api/response"
	"autoadmin/internal/identity"
	"autoadmin/internal/shared/apperror"

	"github.com/gin-gonic/gin"
)

func Authenticate(tokens *identity.TokenManager) gin.HandlerFunc {
	return func(context *gin.Context) {
		rawToken := strings.TrimSpace(context.GetHeader("Authorization"))
		if rawToken == "" {
			rawToken = strings.TrimSpace(context.Query("token"))
		}
		claims, err := tokens.Parse(rawToken)
		if err != nil {
			appError := apperror.ErrTokenInvalid
			if identity.IsTokenExpired(err) {
				appError = apperror.ErrTokenExpired
			}
			response.Error(context, appError)
			context.Abort()
			return
		}
		context.Set(identity.ClaimsContextKey, claims)
		context.Next()
	}
}

func RequirePermission(permission string) gin.HandlerFunc {
	return func(context *gin.Context) {
		claims, ok := identity.ClaimsFromContext(context)
		if !ok {
			response.Error(context, apperror.ErrTokenInvalid)
			context.Abort()
			return
		}
		if claims.Username == "admin" || contains(claims.Perms, permission) {
			context.Next()
			return
		}
		response.Error(context, apperror.ErrPermissionDenied)
		context.Abort()
	}
}

func contains(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}
