package main

import (
	"github.com/KasumiYuku/Aurorix/lib/bot"

	_ "github.com/KasumiYuku/Aurorix/plugins/bind"
	_ "github.com/KasumiYuku/Aurorix/plugins/echo"
	_ "github.com/KasumiYuku/Aurorix/plugins/imagegen"
	_ "github.com/KasumiYuku/Aurorix/plugins/kurocos"
	_ "github.com/KasumiYuku/Aurorix/plugins/uptime"
	_ "github.com/KasumiYuku/Aurorix/plugins/gscore"
)

func main() {
	bot.Run()
}
