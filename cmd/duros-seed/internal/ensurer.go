package durosensurer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"
)

type (
	Ensurer struct {
		log *slog.Logger
		api durosv2.DurosAPIClient
	}
)

func NewEnsurer(log *slog.Logger, api durosv2.DurosAPIClient) *Ensurer {
	return &Ensurer{
		log: log,
		api: api,
	}
}

func (e *Ensurer) EnsurePolicies(ctx context.Context, want []QoSPolicyDef) error {
	existingPoliciesResp, err := e.api.ListPolicies(ctx, &durosv2.ListPoliciesRequest{})
	if err != nil {
		return err
	}
	existingByName := make(map[string]*durosv2.Policy)
	for _, p := range existingPoliciesResp.GetPolicies() {
		if !isGlobalPolicy(p) {
			continue
		}
		existingByName[p.GetName()] = p
	}

	var errs []error
	for _, p := range want {
		old, ok := existingByName[p.Name]
		if ok {
			delete(existingByName, p.Name)
		}
		err := e.ensurePolicy(ctx, p, old)
		if err != nil {
			errs = append(errs, err)
		}
	}

	for _, p := range existingByName {
		_, err := e.api.DeletePolicy(ctx, &durosv2.DeletePolicyRequest{
			UUID: p.GetUUID(),
			Name: p.GetName(),
		})
		if err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (e *Ensurer) ensurePolicy(ctx context.Context, new QoSPolicyDef, old *durosv2.Policy) error {
	rateLimit, err := policyQoSRateLimitFromDef(new)
	if err != nil {
		return err
	}
	if old == nil {
		_, err = e.api.CreatePolicy(ctx, &durosv2.CreatePolicyRequest{
			Name:        new.Name,
			Description: new.Description,
			Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
				QoSRateLimitPolicy: rateLimit,
			},
		})
		return err
	}
	_, err = e.api.UpdatePolicy(ctx, &durosv2.UpdatePolicyRequest{
		UUID:        old.GetUUID(),
		Name:        old.GetName(),
		Description: new.Description,
		Policy: &durosv2.UpdatePolicyRequest_QoSRateLimitPolicy{
			QoSRateLimitPolicy: rateLimit,
		},
	})
	return err
}

func policyQoSRateLimitFromDef(def QoSPolicyDef) (*durosv2.QoSRateLimitPolicy, error) {
	rateLimit := &durosv2.QoSRateLimitPolicy{
		PolicyVisibility: durosv2.PolicyVisibility_Global,
		QoSLimit:         nil,
	}

	switch {
	case def.Limit.Bandwidth != nil:
		rateLimit.QoSLimit = &durosv2.QoSRateLimitPolicy_LimitBw{
			LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
				WriteBWLimit: def.Limit.Bandwidth.Write,
				ReadBWLimit:  def.Limit.Bandwidth.Read,
			},
		}
	case def.Limit.IOPS != nil:
		rateLimit.QoSLimit = &durosv2.QoSRateLimitPolicy_LimitIOPS{
			LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
				WriteIOPSLimit: def.Limit.IOPS.Write,
				ReadIOPSLimit:  def.Limit.IOPS.Read,
			},
		}
	case def.Limit.IOPSPerGB != nil:
		rateLimit.QoSLimit = &durosv2.QoSRateLimitPolicy_LimitIOPSPerGB{
			LimitIOPSPerGB: &durosv2.QoSRateLimitPolicy_QoSLimitIOPSPerGB{
				WriteIOPSPerGBLimit: def.Limit.IOPSPerGB.Write,
				ReadIOPSPerGBLimit:  def.Limit.IOPSPerGB.Read,
			},
		}
	default:
		return nil, fmt.Errorf("missing limit for qos policy %q", def.Name)
	}

	return rateLimit, nil
}

func isGlobalPolicy(p *durosv2.Policy) bool {
	limit, ok := p.GetInfo().(*durosv2.Policy_QoSRateLimitPolicy)
	if !ok {
		return false
	}
	return limit.QoSRateLimitPolicy.GetPolicyVisibility() == durosv2.PolicyVisibility_Global
}
