/*
Copyright 2022 Acnodal.
*/

package v1

const (
	// User-visible annotations

	// EPICSharingKeyAnnotation is applied to a Gateway resource when
	// that resource is a shared Gateway. This means that the EPIC
	// Gateway client will connect to that Gateway instead of creating a
	// new one.
	EPICSharingKeyAnnotation = "acnodal.io/epic-sharing-key"

	// Internal annotations that probably aren't useful to users.

	// EPICLinkAnnotation stores the link to the corresponding resource
	// on the EPIC system.
	EPICLinkAnnotation string = "acnodal.io/epic-link"
	// EPICLinkAnnotationPatch is the EPICLinkAnnotation encoded so it
	// can be used in a JSON patch
	EPICLinkAnnotationPatch string = "/metadata/annotations/acnodal.io~1epic-link"

	// EPICConfigAnnotation stores the config to the corresponding resource
	// on the EPIC system.
	EPICConfigAnnotation string = "acnodal.io/epic-config"
	// EPICConfigAnnotationPatch is the EPICConfigAnnotation encoded so it
	// can be used in a JSON patch
	EPICConfigAnnotationPatch string = "/metadata/annotations/acnodal.io~1epic-config"
)
