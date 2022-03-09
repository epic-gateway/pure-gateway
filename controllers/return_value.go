/*
Copyright 2022 Acnodal.
*/

package controllers

import (
	"time"

	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	// Done tells the controller manager that everything is OK.
	Done = ctrl.Result{Requeue: false}

	// TryAgain asks the controller manager to retry.
	TryAgain = ctrl.Result{RequeueAfter: 10 * time.Second}

	// TryAgainLater asks the controller manager to retry.
	TryAgainLater = ctrl.Result{RequeueAfter: 30 * time.Second}
)
