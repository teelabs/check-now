package completion

import (
	_ "embed"
)

//go:embed scripts/bash
var bashScript string

//go:embed scripts/zsh
var zshScript string

//go:embed scripts/fish
var fishScript string

//go:embed scripts/powershell
var powershellScript string

var scriptData = map[string]string{
	"bash":       bashScript,
	"zsh":        zshScript,
	"fish":       fishScript,
	"powershell": powershellScript,
}
