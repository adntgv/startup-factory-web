package main

import "embed"

//go:embed web
var webFS embed.FS

//go:embed migrations
var migrationsFS embed.FS
