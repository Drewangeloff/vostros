package vostros

import "embed"

//go:embed all:web/templates
var TemplateFS embed.FS

//go:embed all:web/static
var StaticFS embed.FS

//go:embed all:migrations
var MigrationsFS embed.FS

// DiscoveryFS serves the same skill used by installers, alongside public API docs.
//
//go:embed skill/SKILL.md all:web/discovery
var DiscoveryFS embed.FS
