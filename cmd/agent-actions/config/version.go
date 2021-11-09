package config

import "fmt"

type AgentActionsVersion struct {
	GitCommit, GitRef, Version string
}

func (a *AgentActionsVersion) String() string {
	return fmt.Sprintf("GitCommit=%q GitRef=%q Version=%q", a.GitCommit, a.GitRef, a.Version)
}
