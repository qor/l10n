package publish

import (
	"net/http"

	"github.com/jinzhu/gorm"
	"github.com/qor/admin"
	"github.com/qor/l10n"
	"github.com/qor/publish"
	"github.com/qor/qor"
)

type availableLocalesInterface interface {
	AvailableLocales() []string
}

type publishableLocalesInterface interface {
	PublishableLocales() []string
}

type editableLocalesInterface interface {
	EditableLocales() []string
}

func getPublishableLocales(req *http.Request, currentUser interface{}) []string {
	if user, ok := currentUser.(publishableLocalesInterface); ok {
		return user.PublishableLocales()
	}

	if user, ok := currentUser.(editableLocalesInterface); ok {
		return user.EditableLocales()
	}

	if user, ok := currentUser.(availableLocalesInterface); ok {
		return user.AvailableLocales()
	}
	return []string{l10n.Global}
}

// RegisterL10nForPublish register l10n language switcher for publish
func RegisterL10nForPublish(Publish *publish.Publish, Admin *admin.Admin) {
	searchHandler := Publish.SearchHandler
	Publish.SearchHandler = func(db *gorm.DB, context *qor.Context) *gorm.DB {
		if context != nil {
			if context.Request != nil && context.Request.URL.Query().Get("locale") == "" {
				publishableLocales := getPublishableLocales(context.Request, context.CurrentUser)
				return searchHandler(db, context).Set("l10n:mode", "unscoped").Where("language_code IN (?)", publishableLocales)
			}
			return searchHandler(db, context).Set("l10n:mode", "locale")
		}
		return searchHandler(db, context).Set("l10n:mode", "unscoped")
	}

	Admin.RegisterViewPath("github.com/qor/l10n/publish/views")

	Admin.RegisterFuncMap("publishable_locales", func(context admin.Context) []string {
		return getPublishableLocales(context.Request, context.CurrentUser)
	})
}
