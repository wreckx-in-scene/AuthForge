package db

import "embed"

//go:embed migrations/tenant/*.sql
var Tenant embed.FS

//go:embed migrations/identity/*.sql
var Identity embed.FS

//go:embed migrations/session/*.sql
var Session embed.FS
