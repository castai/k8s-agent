package config

import "fmt"

var VersionInfo *AgentVersion

type AgentVersion struct {
	GitCommit, GitRef, Version string
}

func (a *AgentVersion) String() string {
	return fmt.Sprintf("GitCommit=%q GitRef=%q Version=%q", a.GitCommit, a.GitRef, a.Version)
}
