package actor_test

import (
	"context"
	"testing"

	"github.com/JustinSornay/go_skeleton/internal/utils/actor"
)

func TestFromDefaultsToSystem(t *testing.T) {
	if got := actor.From(context.Background()); got != actor.System {
		t.Fatalf("got %+v, want %+v", got, actor.System)
	}
}

func TestRoundTrip(t *testing.T) {
	for _, want := range []actor.Actor{actor.User("u-1"), actor.Cron("retention")} {
		if got := actor.From(actor.With(context.Background(), want)); got != want {
			t.Errorf("got %+v, want %+v", got, want)
		}
	}
}
