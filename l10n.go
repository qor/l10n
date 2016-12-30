package l10n

import (
	"fmt"
	"net/http"
	"reflect"
	"regexp"

	"github.com/qor/admin"
	"github.com/qor/qor"
	"github.com/qor/qor/resource"
	"github.com/qor/qor/utils"
	"github.com/qor/roles"
)

// Global global language
var Global = "en-US"

type l10nInterface interface {
	IsGlobal() bool
	SetLocale(locale string)
}

// Locale embed this struct into GROM-backend models to enable localization feature for your model
type Locale struct {
	LanguageCode string `sql:"size:20" gorm:"primary_key"`
}

// IsGlobal return if current locale is global
func (l Locale) IsGlobal() bool {
	return l.LanguageCode == Global
}

// SetLocale set model's locale
func (l *Locale) SetLocale(locale string) {
	l.LanguageCode = locale
}

// LocaleCreatable if you embed it into your model, it will make the resource be creatable from locales, by default, you can only create it from global
type LocaleCreatable struct {
	Locale
}

// LocaleCreatable a method to allow your mod=el be creatable from locales
func (LocaleCreatable) CreatableFromLocale() {}

type availableLocalesInterface interface {
	AvailableLocales() []string
}

type viewableLocalesInterface interface {
	ViewableLocales() []string
}

type editableLocalesInterface interface {
	EditableLocales() []string
}

func getAvailableLocales(req *http.Request, currentUser interface{}) []string {
	if user, ok := currentUser.(viewableLocalesInterface); ok {
		return user.ViewableLocales()
	}

	if user, ok := currentUser.(availableLocalesInterface); ok {
		return user.AvailableLocales()
	}
	return []string{Global}
}

func getEditableLocales(req *http.Request, currentUser interface{}) []string {
	if user, ok := currentUser.(editableLocalesInterface); ok {
		return user.EditableLocales()
	}

	if user, ok := currentUser.(availableLocalesInterface); ok {
		return user.AvailableLocales()
	}
	return []string{Global}
}

func getLocaleFromContext(context *qor.Context) string {
	if locale := utils.GetLocale(context); locale != "" {
		return locale
	}

	return Global
}

// ConfigureQorResource configure qor locale for Qor Admin
func (l *Locale) ConfigureQorResource(res resource.Resourcer) {
	if res, ok := res.(*admin.Resource); ok {
		Admin := res.GetAdmin()
		res.UseTheme("l10n")

		if res.Permission == nil {
			res.Permission = roles.NewPermission()
		}
		res.Permission.Allow(roles.CRUD, "locale_admin").Allow(roles.Read, "locale_reader")

		if res.GetMeta("Localization") == nil {
			res.Meta(&admin.Meta{Name: "Localization", Type: "localization", Valuer: func(value interface{}, ctx *qor.Context) interface{} {
				var languageCodes []string
				var db = ctx.GetDB()
				var scope = db.NewScope(value)
				db.New().Set("l10n:mode", "unscoped").Model(res.Value).Where(fmt.Sprintf("%v = ?", scope.PrimaryKey()), scope.PrimaryKeyValue()).Pluck("DISTINCT language_code", &languageCodes)
				return languageCodes
			}})
		}

		var attrs = res.ConvertSectionToStrings(res.IndexAttrs())
		var hasLocalization bool
		for _, attr := range attrs {
			if attr == "Localization" {
				hasLocalization = true
				break
			}
		}

		if hasLocalization {
			res.IndexAttrs(res.IndexAttrs(), "-LanguageCode")
		} else {
			res.IndexAttrs(res.IndexAttrs(), "-LanguageCode", "Localization")
		}
		res.NewAttrs(res.NewAttrs(), "-LanguageCode", "-Localization")
		res.EditAttrs(res.EditAttrs(), "-LanguageCode", "-Localization")
		res.ShowAttrs(res.ShowAttrs(), "-LanguageCode", "-Localization", false)

		// Set meta permissions
		for _, field := range Admin.Config.DB.NewScope(res.Value).Fields() {
			if isSyncField(field.StructField) {
				if meta := res.GetMeta(field.Name); meta != nil {
					permission := meta.Meta.Permission
					if permission == nil {
						permission = roles.Allow(roles.CRUD, "global_admin").Allow(roles.Read, "locale_reader")
					} else {
						permission = permission.Allow(roles.CRUD, "global_admin").Allow(roles.Read, "locale_reader")
					}

					meta.SetPermission(permission)
				} else {
					res.Meta(&admin.Meta{Name: field.Name, Permission: roles.Allow(roles.CRUD, "global_admin").Allow(roles.Read, "locale_reader")})
				}
			}
		}

		// Roles
		role := res.Permission.Role
		if _, ok := role.Get("global_admin"); !ok {
			role.Register("global_admin", func(req *http.Request, currentUser interface{}) bool {
				if getLocaleFromContext(&qor.Context{Request: req}) == Global {
					for _, locale := range getEditableLocales(req, currentUser) {
						if locale == Global {
							return true
						}
					}
				}
				return false
			})
		}

		if _, ok := role.Get("locale_admin"); !ok {
			role.Register("locale_admin", func(req *http.Request, currentUser interface{}) bool {
				currentLocale := getLocaleFromContext(&qor.Context{Request: req})
				for _, locale := range getEditableLocales(req, currentUser) {
					if locale == currentLocale {
						return true
					}
				}
				return false
			})
		}

		if _, ok := role.Get("locale_reader"); !ok {
			role.Register("locale_reader", func(req *http.Request, currentUser interface{}) bool {
				currentLocale := getLocaleFromContext(&qor.Context{Request: req})
				for _, locale := range getAvailableLocales(req, currentUser) {
					if locale == currentLocale {
						return true
					}
				}
				return false
			})
		}

		// Inject for l10n
		Admin.RegisterViewPath("github.com/qor/l10n/views")

		// Middleware
		Admin.GetRouter().Use(&admin.Middleware{
			Name: "l10n_set_locale",
			Handler: func(context *admin.Context, middleware *admin.Middleware) {
				db := context.GetDB().Set("l10n:locale", getLocaleFromContext(context.Context))
				if mode := context.Request.URL.Query().Get("locale_mode"); mode != "" {
					db = db.Set("l10n:mode", mode)
				}

				for key, values := range context.Request.URL.Query() {
					if regexp.MustCompile(`primary_key\[.+_language_code\]`).MatchString(key) {
						if len(values) > 0 {
							db = db.Set("l10n:locale", values[0])

							// PUT usually used for localize
							if context.Request.Method == "PUT" {
								db = db.Set(key, "")
								db = db.Set("l10n:localize_to", getLocaleFromContext(context.Context))
							}
						}
					}
				}

				if context.Request.URL.Query().Get("sorting") != "" {
					db = db.Set("l10n:mode", "locale")
				}
				context.SetDB(db)

				middleware.Next(context)
			},
		})

		// FunMap
		Admin.RegisterFuncMap("current_locale", func(context admin.Context) string {
			return getLocaleFromContext(context.Context)
		})

		Admin.RegisterFuncMap("global_locale", func() string {
			return Global
		})

		Admin.RegisterFuncMap("viewable_locales", func(context admin.Context) []string {
			return getAvailableLocales(context.Request, context.CurrentUser)
		})

		Admin.RegisterFuncMap("editable_locales", func(context admin.Context) []string {
			return getEditableLocales(context.Request, context.CurrentUser)
		})

		Admin.RegisterFuncMap("createable_locales", func(context admin.Context) []string {
			editableLocales := getEditableLocales(context.Request, context.CurrentUser)
			if _, ok := context.Resource.Value.(localeCreatableInterface); ok {
				return editableLocales
			}

			for _, locale := range editableLocales {
				if locale == Global {
					return []string{Global}
				}
			}
			return []string{}
		})

		if res.GetAction("Localize") == nil {
			type actionArgument struct {
				From string
				To   string
			}
			argumentResource := Admin.NewResource(&actionArgument{})
			argumentResource.Meta(&admin.Meta{
				Name: "From",
				Type: "select_one",
				Valuer: func(_ interface{}, context *qor.Context) interface{} {
					return Global
				},
				Collection: func(value interface{}, context *qor.Context) (results [][]string) {
					for _, locale := range getAvailableLocales(context.Request, context.CurrentUser) {
						results = append(results, []string{locale, locale})
					}
					return
				},
			})
			argumentResource.Meta(&admin.Meta{
				Name: "To",
				Type: "select_one",
				Valuer: func(_ interface{}, context *qor.Context) interface{} {
					return getLocaleFromContext(context)
				},
				Collection: func(value interface{}, context *qor.Context) (results [][]string) {
					for _, locale := range getEditableLocales(context.Request, context.CurrentUser) {
						results = append(results, []string{locale, locale})
					}
					return
				},
			})

			res.Action(&admin.Action{
				Name: "Localize",
				Handle: func(argument *admin.ActionArgument) error {
					db := argument.Context.GetDB()
					arg := argument.Argument.(*actionArgument)
					results := res.NewSlice()

					db.Set("l10n:locale", arg.From).Find(results, fmt.Sprintf("%v IN (?)", res.PrimaryDBName()), argument.PrimaryValues)

					reflectResults := reflect.Indirect(reflect.ValueOf(results))
					for i := 0; i < reflectResults.Len(); i++ {
						if err := db.Set("l10n:locale", arg.To).Save(reflectResults.Index(i).Interface()).Error; err != nil {
							return err
						}
					}
					return nil
				},
				Modes:      []string{"index", "menu_item"},
				Permission: roles.Allow(roles.CRUD, roles.Anyone),
				Resource:   argumentResource,
			})
		}
	}
}
