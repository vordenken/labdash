package server

import (
	"errors"
	"net/url"
	"strings"
)

// containerLinkFields are the placeholders a container link template may use.
var containerLinkFields = strings.NewReplacer("{id}", "x", "{name}", "x", "{project}", "x")

// ValidateContainerLink checks a LABDASH_CONTAINER_LINK template such as
// "https://arcane.example.org/containers/{id}?env=0". It must be an absolute
// http(s) URL and may only use the placeholders {id}, {name} and {project}.
// An empty template is valid and disables the link.
func ValidateContainerLink(tmpl string) error {
	if tmpl == "" {
		return nil
	}
	probe := containerLinkFields.Replace(tmpl)
	if strings.ContainsAny(probe, "{}") {
		return errors.New("unknown placeholder; use {id}, {name} or {project}")
	}
	u, err := url.Parse(probe)
	if err != nil {
		return err
	}
	if (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("must be an absolute http or https URL")
	}
	return nil
}
