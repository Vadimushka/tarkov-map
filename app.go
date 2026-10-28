package main

import "context"

// App — методы, доступные фронтенду через window.go.main.App.
type App struct {
	ctx context.Context
}

func NewApp() *App {
	return &App{}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// Ping проверяет, что фронтенд достаёт до бэкенда.
func (a *App) Ping() string {
	return "pong"
}
