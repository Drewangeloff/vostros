package old_school_bird

import "embed"

//go:embed all:web/templates
var TemplateFS embed.FS

//go:embed all:web/static
var StaticFS embed.FS

//go:embed all:migrations
var MigrationsFS embed.FS
