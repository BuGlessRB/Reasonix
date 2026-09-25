package config

import "slices"

// heldSandbox is the user's sandbox grants, held across the project merge.
// The slices are copies: decoding the project file writes into the arrays the
// user's decode left behind, so a shared one would read back the repo's list.
type heldSandbox struct {
	hostAuthorities, allowedDomains, deniedDomains []string
}

func holdUserSandbox(s SandboxConfig) heldSandbox {
	return heldSandbox{
		hostAuthorities: slices.Clone(s.HostAuthorities),
		allowedDomains:  slices.Clone(s.AllowedDomains),
		deniedDomains:   slices.Clone(s.DeniedDomains),
	}
}

// restore puts back what a repo may not change. A cloned repo must not hand
// itself the container daemon socket. An egress allow list only narrows open
// network, so a repo may set one the user has not, but never replace the
// user's; its denials only add to the user's.
func (h heldSandbox) restore(s *SandboxConfig) {
	s.HostAuthorities = h.hostAuthorities
	if len(h.allowedDomains) > 0 {
		s.AllowedDomains = h.allowedDomains
	}
	denied := h.deniedDomains
	for _, d := range s.DeniedDomains {
		if !slices.Contains(denied, d) {
			denied = append(denied, d)
		}
	}
	s.DeniedDomains = denied
}
