package subcommands

import (
	"fmt"
	"path/filepath"
	"sort"

	"github.com/PlakarKorp/plakar/agent"
	"github.com/PlakarKorp/plakar/appcontext"
	"github.com/PlakarKorp/plakar/repository"
)

var subcommands map[string]func(*appcontext.AppContext, *repository.Repository, []string) (int, error) = make(map[string]func(*appcontext.AppContext, *repository.Repository, []string) (int, error))

func Register(command string, fn func(*appcontext.AppContext, *repository.Repository, []string) (int, error)) {
	subcommands[command] = fn
}

func Execute(ctx *appcontext.AppContext, repo *repository.Repository, command string, args []string, agentless bool) (int, error) {
	if agentless {
		fn, exists := subcommands[command]
		if !exists {
			return 1, fmt.Errorf("unknown command: %s", command)
		}
		return fn(ctx, repo, args)
	}

	client, err := agent.NewClient(filepath.Join(ctx.CacheDir, "agent.sock"))
	if err != nil {
		return 1, err
	}
	defer client.Close()

	return client.SendCommand(ctx, repo.Location(), command, args)
}

func List() []string {
	var list []string
	for command := range subcommands {
		list = append(list, command)
	}
	sort.Strings(list)
	return list
}
