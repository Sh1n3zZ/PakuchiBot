package handler

import (
	zero "github.com/wdvxdr1123/ZeroBot"
)

func RegisterPingHandler() {
	zero.OnCommand("ping").Handle(func(ctx *zero.Ctx) {
		ctx.Send("pong!")
	})
}
