package durosensurer_test

import (
	"context"
	"log/slog"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	durosensurer "github.com/metal-stack/duros-controller/cmd/duros-seed/internal"
	durosv2 "github.com/metal-stack/duros-go/api/duros/v2"
	durosv2mocks "github.com/metal-stack/duros-go/api/duros/v2/mocks"
)

func TestEnsurePolicies(t *testing.T) {
	tests := []struct {
		name     string
		policies []durosensurer.QoSPolicyDef
		wantErr  string
		prepare  func(t *testing.T, api *durosv2mocks.DurosAPIClient)
	}{
		{
			name:     "does nothing if no policy defs given and no policies exist",
			policies: []durosensurer.QoSPolicyDef{},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{},
					}, nil)
			},
		},
		{
			name:     "does not delete policies if no policy defs given",
			policies: []durosensurer.QoSPolicyDef{},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{
							{
								UUID:        "uuid",
								Name:        "no-limit-policy",
								Description: "Initial global default policy",
							},
						},
					}, nil)
			},
		},
		{
			name: "creates policy if not exists",
			policies: []durosensurer.QoSPolicyDef{
				{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Limit: durosensurer.QoSPolicyLimit{
						Bandwidth: &durosensurer.QoSPolicyLimitBandwidth{
							Read:  15,
							Write: 15,
						},
					},
				},
			},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{},
					}, nil)

				api.On("CreatePolicy", mock.Anything, mock.MatchedBy(equalsCreation(t, &durosv2.CreatePolicyRequest{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
								LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
									WriteBWLimit: 15,
									ReadBWLimit:  15,
								},
							},
						},
					},
				}))).
					Return(&durosv2.Policy{
						UUID:        "floppy-id",
						Name:        "floppy-policy",
						Description: "Unbarebly slow",
						State:       durosv2.Policy_Creating,
						Info:        nil,
					}, nil)
			},
		},
		{
			name: "updates existing policy",
			policies: []durosensurer.QoSPolicyDef{
				{
					Name:        "floppy-policy",
					Description: "Now blazingly fast",
					Limit: durosensurer.QoSPolicyLimit{
						Bandwidth: &durosensurer.QoSPolicyLimitBandwidth{
							Read:  30,
							Write: 30,
						},
					},
				},
			},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{
							{
								UUID:        "floppy-id",
								Name:        "floppy-policy",
								Description: "Unbarebly slow",
								State:       durosv2.Policy_Active,
								Info: &durosv2.Policy_QoSRateLimitPolicy{
									QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
										PolicyVisibility: durosv2.PolicyVisibility_Global,
										QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
											LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
												WriteBWLimit: 15,
												ReadBWLimit:  15,
											},
										},
									},
								},
							},
						},
					}, nil)

				api.On("UpdatePolicy", mock.Anything, mock.MatchedBy(equalsUpdate(t, &durosv2.UpdatePolicyRequest{
					UUID:        "floppy-id",
					Name:        "floppy-policy",
					Description: "Now blazingly fast",
					Policy: &durosv2.UpdatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
								LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
									WriteBWLimit: 30,
									ReadBWLimit:  30,
								},
							},
						},
					},
				}))).
					Return(&durosv2.UpdatePolicyResponse{}, nil)
			},
		},
		{
			name: "creates policy if not exists and ignores non-global policies",
			policies: []durosensurer.QoSPolicyDef{
				{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Limit: durosensurer.QoSPolicyLimit{
						Bandwidth: &durosensurer.QoSPolicyLimitBandwidth{
							Read:  15,
							Write: 15,
						},
					},
				},
			},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{
							{
								UUID:        "ignored",
								Name:        "customer-policy",
								Description: "This policy is project specific",
								State:       durosv2.Policy_Active,
								Info: &durosv2.Policy_QoSRateLimitPolicy{
									QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
										ProjectsNamesScope: []string{"my-proj"},
										PolicyVisibility:   durosv2.PolicyVisibility_Scoped,
										QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPS{
											LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
												WriteIOPSLimit: 0,
												ReadIOPSLimit:  0,
											},
										},
									},
								},
							},
						},
					}, nil)

				api.On("CreatePolicy", mock.Anything, mock.MatchedBy(equalsCreation(t, &durosv2.CreatePolicyRequest{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
								LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
									WriteBWLimit: 15,
									ReadBWLimit:  15,
								},
							},
						},
					},
				}))).
					Return(&durosv2.Policy{
						UUID:        "floppy-id",
						Name:        "floppy-policy",
						Description: "Unbarebly slow",
						State:       durosv2.Policy_Creating,
						Info:        nil,
					}, nil)
			},
		},
		{
			name: "creates policy if not exists and deletes other global policies",
			policies: []durosensurer.QoSPolicyDef{
				{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Limit: durosensurer.QoSPolicyLimit{
						Bandwidth: &durosensurer.QoSPolicyLimitBandwidth{
							Read:  15,
							Write: 15,
						},
					},
				},
			},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{
							{
								UUID:        "other-id",
								Name:        "other-policy",
								Description: "This policy is about to be deleted",
								State:       durosv2.Policy_Active,
								Info: &durosv2.Policy_QoSRateLimitPolicy{
									QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
										PolicyVisibility: durosv2.PolicyVisibility_Global,
										QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPS{
											LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
												WriteIOPSLimit: 0,
												ReadIOPSLimit:  0,
											},
										},
									},
								},
							},
						},
					}, nil)

				api.On("CreatePolicy", mock.Anything, mock.MatchedBy(equalsCreation(t, &durosv2.CreatePolicyRequest{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
								LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
									WriteBWLimit: 15,
									ReadBWLimit:  15,
								},
							},
						},
					},
				}))).
					Return(&durosv2.Policy{
						UUID:        "floppy-id",
						Name:        "floppy-policy",
						Description: "Unbarebly slow",
						State:       durosv2.Policy_Creating,
						Info:        nil,
					}, nil)

				api.On("DeletePolicy", mock.Anything, &durosv2.DeletePolicyRequest{
					UUID: "other-id",
					Name: "other-policy",
				}).
					Return(&durosv2.DeletePolicyResponse{}, nil)
			},
		},
		{
			name: "complex example",
			policies: []durosensurer.QoSPolicyDef{
				{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Limit: durosensurer.QoSPolicyLimit{
						Bandwidth: &durosensurer.QoSPolicyLimitBandwidth{
							Read:  20,
							Write: 15,
						},
					},
				},
				{
					Name:        "much-iops",
					Description: "So fast",
					Limit: durosensurer.QoSPolicyLimit{
						IOPS: &durosensurer.QoSPolicyLimitIOPS{
							Read:  5000,
							Write: 4800,
						},
					},
				},
				{
					Name:        "adaptable",
					Description: "The bigger, the faster",
					Limit: durosensurer.QoSPolicyLimit{
						IOPSPerGB: &durosensurer.QoSPolicyLimitIOPSPerGB{
							Read:  100,
							Write: 90,
						},
					},
				},
			},
			prepare: func(t *testing.T, api *durosv2mocks.DurosAPIClient) {
				t.Helper()
				api.On("ListPolicies", mock.Anything, &durosv2.ListPoliciesRequest{}).
					Return(&durosv2.ListPoliciesResponse{
						Policies: []*durosv2.Policy{
							{
								UUID:        "other-id",
								Name:        "other-policy",
								Description: "This policy is about to be deleted",
								State:       durosv2.Policy_Active,
								Info: &durosv2.Policy_QoSRateLimitPolicy{
									QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
										PolicyVisibility: durosv2.PolicyVisibility_Global,
										QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPS{
											LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
												WriteIOPSLimit: 0,
												ReadIOPSLimit:  0,
											},
										},
									},
								},
							},
							{
								UUID:        "adaptable-id",
								Name:        "adaptable",
								Description: "The bigger, the faster",
								State:       durosv2.Policy_Active,
								Info: &durosv2.Policy_QoSRateLimitPolicy{
									QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
										PolicyVisibility: durosv2.PolicyVisibility_Global,
										QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPSPerGB{
											LimitIOPSPerGB: &durosv2.QoSRateLimitPolicy_QoSLimitIOPSPerGB{
												WriteIOPSPerGBLimit: 85,
												ReadIOPSPerGBLimit:  80,
											},
										}},
								},
							},
						},
					}, nil)

				api.On("CreatePolicy", mock.Anything, mock.MatchedBy(equalsCreation(t, &durosv2.CreatePolicyRequest{
					Name:        "floppy-policy",
					Description: "Unbarebly slow",
					Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
								LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
									WriteBWLimit: 15,
									ReadBWLimit:  20,
								},
							},
						},
					},
				}))).
					Once().
					Return(&durosv2.Policy{
						UUID:        "floppy-id",
						Name:        "floppy-policy",
						Description: "Unbarebly slow",
						State:       durosv2.Policy_Creating,
						Info: &durosv2.Policy_QoSRateLimitPolicy{
							QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
								QoSLimit: &durosv2.QoSRateLimitPolicy_LimitBw{
									LimitBw: &durosv2.QoSRateLimitPolicy_QoSLimitBW{
										WriteBWLimit: 15,
										ReadBWLimit:  20,
									},
								},
							},
						},
					}, nil)

				api.On("CreatePolicy", mock.Anything, mock.MatchedBy(equalsCreation(t, &durosv2.CreatePolicyRequest{
					Name:        "much-iops",
					Description: "So fast",
					Policy: &durosv2.CreatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPS{
								LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
									WriteIOPSLimit: 4800,
									ReadIOPSLimit:  5000,
								},
							}},
					},
				}))).
					Once().
					Return(&durosv2.Policy{
						UUID:        "much-iops-id",
						Name:        "much-iops",
						Description: "So fast",
						State:       durosv2.Policy_Creating,
						Info: &durosv2.Policy_QoSRateLimitPolicy{
							QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
								PolicyVisibility: durosv2.PolicyVisibility_Global,
								QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPS{
									LimitIOPS: &durosv2.QoSRateLimitPolicy_QoSLimitIOPS{
										WriteIOPSLimit: 4864,
										ReadIOPSLimit:  5210,
									},
								},
							},
						},
					}, nil)

				api.On("UpdatePolicy", mock.Anything, mock.MatchedBy(equalsUpdate(t, &durosv2.UpdatePolicyRequest{
					UUID:        "adaptable-id",
					Name:        "adaptable",
					Description: "The bigger, the faster",
					Policy: &durosv2.UpdatePolicyRequest_QoSRateLimitPolicy{
						QoSRateLimitPolicy: &durosv2.QoSRateLimitPolicy{
							PolicyVisibility: durosv2.PolicyVisibility_Global,
							QoSLimit: &durosv2.QoSRateLimitPolicy_LimitIOPSPerGB{
								LimitIOPSPerGB: &durosv2.QoSRateLimitPolicy_QoSLimitIOPSPerGB{
									WriteIOPSPerGBLimit: 90,
									ReadIOPSPerGBLimit:  100,
								},
							}},
					},
				}))).
					Once().
					Return(&durosv2.UpdatePolicyResponse{}, nil)

				api.On("DeletePolicy", mock.Anything, &durosv2.DeletePolicyRequest{
					UUID: "other-id",
					Name: "other-policy",
				}).
					Return(&durosv2.DeletePolicyResponse{}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			apiMock := durosv2mocks.NewDurosAPIClient(t)
			tt.prepare(t, apiMock)
			ensurer := durosensurer.NewEnsurer(slog.Default(), apiMock)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			err := ensurer.EnsurePolicies(ctx, tt.policies)
			if tt.wantErr != "" {
				assert.EqualError(t, err, tt.wantErr)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func equalsCreation(t *testing.T, want *durosv2.CreatePolicyRequest) func(got *durosv2.CreatePolicyRequest) bool {
	t.Helper()
	return func(got *durosv2.CreatePolicyRequest) bool {
		t.Helper()
		return objectsExportedFieldsAreEqual(want, got)
	}
}

func equalsUpdate(t *testing.T, want *durosv2.UpdatePolicyRequest) func(got *durosv2.UpdatePolicyRequest) bool {
	t.Helper()
	return func(got *durosv2.UpdatePolicyRequest) bool {
		t.Helper()
		return objectsExportedFieldsAreEqual(want, got)
	}
}

func objectsExportedFieldsAreEqual(expected, actual interface{}) bool {
	expectedCleaned := copyExportedFields(expected)
	actualCleaned := copyExportedFields(actual)
	return assert.ObjectsAreEqualValues(expectedCleaned, actualCleaned)
}

func copyExportedFields(expected interface{}) interface{} {
	if isNil(expected) {
		return expected
	}

	expectedType := reflect.TypeOf(expected)
	expectedKind := expectedType.Kind()
	expectedValue := reflect.ValueOf(expected)

	switch expectedKind {
	case reflect.Struct:
		result := reflect.New(expectedType).Elem()
		for i := 0; i < expectedType.NumField(); i++ {
			field := expectedType.Field(i)
			isExported := field.IsExported()
			if isExported {
				fieldValue := expectedValue.Field(i)
				if isNil(fieldValue) || isNil(fieldValue.Interface()) {
					continue
				}
				newValue := copyExportedFields(fieldValue.Interface())
				result.Field(i).Set(reflect.ValueOf(newValue))
			}
		}
		return result.Interface()

	case reflect.Ptr:
		result := reflect.New(expectedType.Elem())
		unexportedRemoved := copyExportedFields(expectedValue.Elem().Interface())
		result.Elem().Set(reflect.ValueOf(unexportedRemoved))
		return result.Interface()

	case reflect.Array, reflect.Slice:
		var result reflect.Value
		if expectedKind == reflect.Array {
			result = reflect.New(reflect.ArrayOf(expectedValue.Len(), expectedType.Elem())).Elem()
		} else {
			result = reflect.MakeSlice(expectedType, expectedValue.Len(), expectedValue.Len())
		}
		for i := 0; i < expectedValue.Len(); i++ {
			index := expectedValue.Index(i)
			if isNil(index) {
				continue
			}
			unexportedRemoved := copyExportedFields(index.Interface())
			result.Index(i).Set(reflect.ValueOf(unexportedRemoved))
		}
		return result.Interface()

	case reflect.Map:
		result := reflect.MakeMap(expectedType)
		for _, k := range expectedValue.MapKeys() {
			index := expectedValue.MapIndex(k)
			unexportedRemoved := copyExportedFields(index.Interface())
			result.SetMapIndex(k, reflect.ValueOf(unexportedRemoved))
		}
		return result.Interface()

	default:
		return expected
	}
}
func isNil(object interface{}) bool {
	if object == nil {
		return true
	}

	value := reflect.ValueOf(object)
	switch value.Kind() {
	case
		reflect.Chan, reflect.Func,
		reflect.Interface, reflect.Map,
		reflect.Ptr, reflect.Slice, reflect.UnsafePointer:

		return value.IsNil()
	}

	return false
}
