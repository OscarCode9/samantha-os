package runtime

import (
	"fmt"
	"net/http"
	"os"

	"github.com/oscarcode/elementary-claw/internal/config"
	"github.com/oscarcode/elementary-claw/internal/session"
	"github.com/oscarcode/elementary-claw/internal/skills"
	"github.com/oscarcode/elementary-claw/internal/tools"
)

func Serve(listenAddr string, paths config.Paths, store *session.Store, registry *tools.Registry, skillsRegistry *skills.Registry) error {
	// Start the skills file watcher for hot-reload.
	watcher := skills.NewWatcher(skillsRegistry, map[skills.Source]string{
		skills.SourceBundled:   paths.BundledSkillsDir,
		skills.SourceManaged:   paths.ManagedSkillsDir,
		skills.SourceWorkspace: paths.WorkspaceSkillsDir,
	}, skills.WatcherOptions{})
	watcher.OnReload = func(count int) {
		fmt.Fprintf(os.Stderr, "skills hot-reloaded: %d skill(s)\n", count)
	}
	watcher.Start()
	defer watcher.Stop()

	server := &http.Server{
		Addr:    listenAddr,
		Handler: newHandler(paths, store, registry, skillsRegistry),
	}

	return fmt.Errorf("gateway server stopped: %w", server.ListenAndServe())
}
