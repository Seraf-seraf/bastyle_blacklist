package logging

import "log"

func Error(methodCtx string, err error) {
	log.Printf("[ERROR]: %s: %s", methodCtx, err)
}

func Panic(methodCtx string, err error) {
	log.Panicf("[ERROR]: %s: %s", methodCtx, err)
}
