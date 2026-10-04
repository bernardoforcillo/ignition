package mailer

import "io/fs"

// TemplateFS exposes the embedded files to the external tests.
func TemplateFS() fs.FS { return templateFS }
