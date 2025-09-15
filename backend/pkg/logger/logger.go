package logger

import (
	"go.uber.org/zap"
)

var L *zap.Logger

func Init() error {
	var err error
	L, err = zap.NewProduction()
	if err != nil {
		return err
	}
	return nil
}

func Sync() {
	if L != nil {
		_ = L.Sync()
	}
}
