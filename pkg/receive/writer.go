// Copyright (c) The Thanos Authors.
// Licensed under the Apache License 2.0.

package receive

import (
	"context"

	"github.com/go-kit/kit/log"
	"github.com/go-kit/kit/log/level"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/thanos-io/thanos/pkg/store/labelpb"

	"github.com/thanos-io/thanos/pkg/errutil"
	"github.com/thanos-io/thanos/pkg/store/storepb/prompb"
)

// Appendable returns an Appender.
type Appendable interface {
	Appender(ctx context.Context) (storage.Appender, error)
}

type TenantStorage interface {
	TenantAppendable(string) (Appendable, error)
}

type Writer struct {
	logger    log.Logger
	multiTSDB TenantStorage
}

func NewWriter(logger log.Logger, multiTSDB TenantStorage) *Writer {
	return &Writer{
		logger:    logger,
		multiTSDB: multiTSDB,
	}
}

func (r *Writer) Write(ctx context.Context, tenantID string, wreq *prompb.WriteRequest) error {
	var (
		numOutOfOrder  = 0
		numDuplicates  = 0
		numOutOfBounds = 0
	)

	s, err := r.multiTSDB.TenantAppendable(tenantID)
	if err != nil {
		return errors.Wrap(err, "get tenant appendable")
	}

	app, err := s.Appender(ctx)
	if err == tsdb.ErrNotReady {
		return err
	}
	if err != nil {
		return errors.Wrap(err, "get appender")
	}
	getRef := app.(storage.GetRef)

	var (
		ref  uint64
		errs errutil.MultiError
	)
	for _, t := range wreq.Timeseries {
		lset := labelpb.ZLabelsToPromLabels(t.Labels)

		// Check if the TSDB has cached reference for those labels.
		ref, lset = getRef.GetRef(lset)
		if ref == 0 {
			// If not, copy labels, as TSDB will hold those strings long term. Given no
			// copy unmarshal we don't want to keep memory for whole protobuf, only for labels.
			labelpb.ReAllocZLabelsStrings(&t.Labels)
			lset = labelpb.ZLabelsToPromLabels(t.Labels)
		}

		// Append as many valid samples as possible, but keep track of the errors.
		for _, s := range t.Samples {
			ref, err = app.Append(ref, lset, s.Timestamp, s.Value)
			switch err {
			case storage.ErrOutOfOrderSample:
				numOutOfOrder++
				level.Debug(r.logger).Log("msg", "Out of order sample", "lset", lset, "sample", s)
			case storage.ErrDuplicateSampleForTimestamp:
				numDuplicates++
				level.Debug(r.logger).Log("msg", "Duplicate sample for timestamp", "lset", lset, "sample", s)
			case storage.ErrOutOfBounds:
				numOutOfBounds++
				level.Debug(r.logger).Log("msg", "Out of bounds metric", "lset", lset, "sample", s)
			}
		}
	}

	if numOutOfOrder > 0 {
		level.Warn(r.logger).Log("msg", "Error on ingesting out-of-order samples", "numDropped", numOutOfOrder)
		errs.Add(errors.Wrapf(storage.ErrOutOfOrderSample, "add %d samples", numOutOfOrder))
	}
	if numDuplicates > 0 {
		level.Warn(r.logger).Log("msg", "Error on ingesting samples with different value but same timestamp", "numDropped", numDuplicates)
		errs.Add(errors.Wrapf(storage.ErrDuplicateSampleForTimestamp, "add %d samples", numDuplicates))
	}
	if numOutOfBounds > 0 {
		level.Warn(r.logger).Log("msg", "Error on ingesting samples that are too old or are too far into the future", "numDropped", numOutOfBounds)
		errs.Add(errors.Wrapf(storage.ErrOutOfBounds, "add %d samples", numOutOfBounds))
	}

	if err := app.Commit(); err != nil {
		errs.Add(errors.Wrap(err, "commit samples"))
	}
	return errs.Err()
}
<<<<<<< HEAD
=======

type fakeTenantAppendable struct {
	f *fakeAppendable
}

func newFakeTenantAppendable(f *fakeAppendable) *fakeTenantAppendable {
	return &fakeTenantAppendable{f: f}
}

func (t *fakeTenantAppendable) TenantAppendable(tenantID string) (Appendable, error) {
	return t.f, nil
}

type fakeAppendable struct {
	appender    storage.Appender
	appenderErr func() error
}

var _ Appendable = &fakeAppendable{}

func nilErrFn() error {
	return nil
}

func (f *fakeAppendable) Appender(_ context.Context) (storage.Appender, error) {
	errf := f.appenderErr
	if errf == nil {
		errf = nilErrFn
	}
	return f.appender, errf()
}

type fakeAppender struct {
	sync.Mutex
	samples     map[uint64][]prompb.Sample
	appendErr   func() error
	commitErr   func() error
	rollbackErr func() error
}

var _ storage.Appender = &fakeAppender{}

func newFakeAppender(appendErr, commitErr, rollbackErr func() error) *fakeAppender { //nolint:unparam
	if appendErr == nil {
		appendErr = nilErrFn
	}
	if commitErr == nil {
		commitErr = nilErrFn
	}
	if rollbackErr == nil {
		rollbackErr = nilErrFn
	}
	return &fakeAppender{
		samples:     make(map[uint64][]prompb.Sample),
		appendErr:   appendErr,
		commitErr:   commitErr,
		rollbackErr: rollbackErr,
	}
}

func (f *fakeAppender) Get(l labels.Labels) []prompb.Sample {
	f.Lock()
	defer f.Unlock()
	s := f.samples[l.Hash()]
	res := make([]prompb.Sample, len(s))
	copy(res, s)
	return res
}

func (f *fakeAppender) Append(ref uint64, l labels.Labels, t int64, v float64) (uint64, error) {
	f.Lock()
	defer f.Unlock()
	if ref == 0 {
		ref = l.Hash()
	}
	f.samples[ref] = append(f.samples[ref], prompb.Sample{Timestamp: t, Value: v})
	return ref, f.appendErr()
}

func (f *fakeAppender) Commit() error {
	return f.commitErr()
}

func (f *fakeAppender) Rollback() error {
	return f.rollbackErr()
}
>>>>>>> 120e2b09... Use new storage.Appender Append() instead of Add & AddFast (#3936)
