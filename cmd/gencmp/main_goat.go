package main

import (
	"github.com/tmr232/goat"
	"github.com/tmr232/goat/flags"
	"github.com/urfave/cli/v2"
)

func init() {
	goat.Register(app, goat.RunConfig{
		Flags: []cli.Flag{
			flags.MakeFlag[string]("dir", "", ""),
		},
		Name:  "app",
		Usage: "",
		Action: func(c *cli.Context) error {
			app(
				flags.GetFlag[string](c, "dir"),
			)
			return nil
		},
		CtxFlagBuilder: func(c *cli.Context) map[string]any {
			cflags := make(map[string]any)
			cflags["dir"] = flags.GetFlag[string](c, "dir")
			return cflags
		},
	})
}
