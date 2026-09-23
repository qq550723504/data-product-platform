package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	annotationinfra "github.com/qq550723504/data-product-platform/apps/platform/internal/annotation/infrastructure"
)

type EngineRunner struct {
	repo       *annotationinfra.Repository
	service    *EngineService
	reconciler *EngineResultReconciler
	workerRef  string
	lease      time.Duration
	batchSize  int
}

func NewEngineRunner(
	repo *annotationinfra.Repository,
	service *EngineService,
	reconciler *EngineResultReconciler,
	workerRef string,
	lease time.Duration,
	batchSize int,
) *EngineRunner {
	if batchSize <= 0 {
		batchSize = 50
	}
	return &EngineRunner{
		repo:       repo,
		service:    service,
		reconciler: reconciler,
		workerRef:  strings.TrimSpace(workerRef),
		lease:      lease,
		batchSize:  batchSize,
	}
}

func (r *EngineRunner) RunOnce(ctx context.Context) error {
	if r == nil || r.repo == nil || r.service == nil || r.reconciler == nil ||
		r.workerRef == "" || r.lease <= 0 {
		return errors.New("annotation engine runner is not configured")
	}

	var runErrs []error
	operationIDs, err := r.repo.ListDispatchableEngineOperationIDs(
		ctx,
		r.service.engine.Provider(),
		r.service.engine.InstanceRef(),
		r.batchSize,
	)
	if err != nil {
		return err
	}
	for _, operationID := range operationIDs {
		if _, err := r.service.Dispatch(ctx, operationID, r.workerRef, r.lease); err != nil {
			runErrs = append(runErrs, fmt.Errorf("dispatch annotation engine operation %s: %w", operationID, err))
		}
	}

	campaignIDs, err := r.repo.ListCampaignIDsNeedingEngineResults(
		ctx,
		r.service.engine.Provider(),
		r.service.engine.InstanceRef(),
		r.batchSize,
	)
	if err != nil {
		runErrs = append(runErrs, err)
	} else {
		for _, campaignID := range campaignIDs {
			if err := r.reconciler.ReconcileCampaign(ctx, campaignID); err != nil {
				runErrs = append(runErrs, fmt.Errorf("reconcile annotation campaign %s: %w", campaignID, err))
			}
		}
	}

	return errors.Join(runErrs...)
}
