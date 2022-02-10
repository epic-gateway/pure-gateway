/*
Copyright 2022 Acnodal.
*/

package v1

const (
	// Internal annotations that probably aren't useful to users.

	// EPICLinkAnnotation stores the link to the corresponding resource
	// on the EPIC system.
	EPICLinkAnnotation string = "acnodal.io/epic-link"
	// EPICLinkAnnotationPatch is the EPICLinkAnnotation encoded so it
	// can be used in a JSON patch
	EPICLinkAnnotationPatch string = "/metadata/annotations/acnodal.io~1epic-link"
)
