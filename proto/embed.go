// Package relkitproto embeds the host-visible updater schema so the engine can
// hand hosts the exact contract it speaks (`relkit-updater proto`, ADR 0026).
// Only relkit.updater.v2 is embedded: engine state is not part of the contract.
package relkitproto

import _ "embed"

//go:embed updater/v2/updater.proto
var UpdaterV2 string
