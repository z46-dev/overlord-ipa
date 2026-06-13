package main

import (
	"github.com/z46-dev/golog"
	"github.com/z46-dev/overlord-ipa/app"
	"github.com/z46-dev/overlord-ipa/conf"
	"github.com/z46-dev/overlord-ipa/db"
)

// main initializes configuration, persistence, and the application server.
func main() {
	var (
		log *golog.Logger = golog.New().Prefix("[OVERLORD IPA]", golog.BoldBlue).Timestamp()
		err error
	)

	if err = conf.Init(); err != nil {
		panic(err)
	}

	if err = db.Init(log); err != nil {
		panic(err)
	}

	if err = app.Init(log); err != nil {
		panic(err)
	}
}
