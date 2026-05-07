package tui

import (
	"errors"
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// RunRepoInput は GitHub の owner/repo を TUI で入力させます。
// デフォルト値がある場合は初期値として使用します。
// ユーザーが中断した場合は ErrAborted を返します。
func RunRepoInput(defaultOwner, defaultRepo string) (owner, repo string, err error) {
	owner = strings.TrimSpace(defaultOwner)
	repo = strings.TrimSpace(defaultRepo)

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("GitHub owner を入力してください").
				Description("例: takiguchi-yu / my-org").
				Value(&owner).
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return fmt.Errorf("owner は必須です")
					}
					return nil
				}),
			huh.NewInput().
				Title("GitHub repository を入力してください").
				Description("例: cording-pilot").
				Value(&repo).
				Validate(func(v string) error {
					if strings.TrimSpace(v) == "" {
						return fmt.Errorf("repository は必須です")
					}
					return nil
				}),
		),
	)

	if runErr := form.Run(); runErr != nil {
		if errors.Is(runErr, huh.ErrUserAborted) {
			return "", "", ErrAborted
		}
		return "", "", fmt.Errorf("tui: repo input form run: %w", runErr)
	}

	owner = strings.TrimSpace(owner)
	repo = strings.TrimSpace(repo)
	return owner, repo, nil
}
