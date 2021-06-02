package config

import "fmt"

type AgentVersion struct {
	GitCommit, GitRef, Version string
}

func (a *AgentVersion) String() string {
	return fmt.Sprintf("GitCommit=%q GitRef=%q Version=%q", a.GitCommit, a.GitRef, a.Version)
}
