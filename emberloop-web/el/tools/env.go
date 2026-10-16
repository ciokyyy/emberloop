package tools

import (
	"context"
	"os"
)

type sessionEnvKey struct{}

func WithSessionEnv(ctx context.Context, env [][2]string) context.Context {
	return context.WithValue(ctx, sessionEnvKey{}, env)
}

func SessionEnv(ctx context.Context) [][2]string {
	env, _ := ctx.Value(sessionEnvKey{}).([][2]string)
	return env
}

func commandEnv(ctx context.Context) []string {
	env := os.Environ()
	for _, pair := range SessionEnv(ctx) {
		env = append(env, pair[0]+"="+pair[1])
	}
	return env
}
