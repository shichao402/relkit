// Package version is the project-side version SSOT for RUP / relkit.
//
// Wire documents (Index / Manifest) carry version+code in protobuf. This package
// owns the on-disk project file that those values come from, so hosts do not
// invent their own VERSION readers.
//
// File: VERSION.json (see FileName)
// Schema: rup.version/1
//
//	{
//	  "schema": "rup.version/1",
//	  "version": "1.2.3+45"
//	}
//
// Prefer the relkit CLI (`relkit version get|set|bump|code`) from any language
// or CI. Import this package only when you are already in Go.
package version
