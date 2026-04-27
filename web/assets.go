package web

import "embed"

//go:embed templates/*.html static static/*
var Assets embed.FS
