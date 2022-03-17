package controllers

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	epicgwv1 "acnodal.io/puregw/apis/puregw/v1"
	"acnodal.io/puregw/internal/acnodal"
)

func ConnectToEPIC(ctx context.Context, cl client.Client, namespace *string, name string) (acnodal.EPIC, error) {
	// Get the PureGW GatewayClassConfig referred to by the GatewayClass
	gwcName := types.NamespacedName{Name: name}
	if namespace != nil {
		gwcName.Namespace = *namespace
	}
	gcc := epicgwv1.GatewayClassConfig{}
	if err := cl.Get(ctx, gwcName, &gcc); err != nil {
		return nil, fmt.Errorf("Unable to get GatewayClassConfig %s", gwcName.String())
	}

	// Connect to EPIC
	epicURL, err := gcc.Spec.EPICAPIServiceURL()
	if err != nil {
		return nil, err
	}
	return acnodal.NewEPIC(epicURL, gcc.Spec.EPIC.SvcAccount, gcc.Spec.EPIC.SvcKey, gcc.Spec.EPIC.ClientName)
}

func SplitNSName(name string) (*types.NamespacedName, error) {
	parts := strings.Split(name, string(types.Separator))
	if len(parts) < 2 {
		return nil, fmt.Errorf("Malformed NamespaceName: %q", parts)
	}

	return &types.NamespacedName{Namespace: parts[0], Name: parts[1]}, nil
}
