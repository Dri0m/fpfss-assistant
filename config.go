package main

import (
	"fmt"
	"github.com/sirupsen/logrus"
	"os"
	"strconv"
)

type Config struct {
	DownloadThreads int64
	Cookie          string
	BaseURL         string
}

func EnvString(name string) string {
	s := os.Getenv(name)
	if s == "" {
		panic(fmt.Sprintf("env variable '%s' is not set", name))
	}
	return s
}

func EnvInt(name string) int64 {
	s := os.Getenv(name)
	if s == "" {
		panic(fmt.Sprintf("env variable '%s' is not set", name))
	}
	i, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		panic(err)
	}
	return i
}

func EnvBool(name string) bool {
	s := os.Getenv(name)
	if s == "" {
		panic(fmt.Sprintf("env variable '%s' is not set", name))
	} else if s == "True" {
		return true
	} else if s == "False" {
		return false
	}
	panic(fmt.Sprintf("invalid value of env variable '%s'", name))
}

func (a *app) getConfig(l *logrus.Logger) {
	l.Infoln("loading config...")

	a.config = &Config{
		DownloadThreads: EnvInt("THREADS"),
		Cookie:          EnvString("COOKIE"),
		BaseURL:         EnvString("BASE_URL"),
	}
}
