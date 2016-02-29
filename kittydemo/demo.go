// demo of kitty

package main

import (
	"github.com/wheelcomplex/hordekit/kitty"
	"github.com/wheelcomplex/hordekit/kitty/logger"
)

func init() {
	println("func init() of program main")
	logger.Panic("kitty demo")
}

func main() {
	kitty.LayerMainInit()
}
