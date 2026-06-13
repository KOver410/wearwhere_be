package domain

// Embed the IANA timezone database so time.LoadLocation works on platforms
// (e.g. Windows) that lack a system zoneinfo database.
import _ "time/tzdata"
