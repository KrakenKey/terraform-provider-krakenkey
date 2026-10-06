package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/terraform-plugin-framework-timeouts/resource/timeouts"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*domainVerificationResource)(nil)
	_ resource.ResourceWithConfigure   = (*domainVerificationResource)(nil)
	_ resource.ResourceWithImportState = (*domainVerificationResource)(nil)
)

const defaultVerifyTimeout = 10 * time.Minute

func NewDomainVerificationResource() resource.Resource {
	return &domainVerificationResource{}
}

type domainVerificationResource struct {
	data *providerData
}

type domainVerificationModel struct {
	ID       types.String   `tfsdk:"id"`
	DomainID types.String   `tfsdk:"domain_id"`
	Verified types.Bool     `tfsdk:"verified"`
	Timeouts timeouts.Value `tfsdk:"timeouts"`
}

func (r *domainVerificationResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_domain_verification"
}

func (r *domainVerificationResource) Schema(ctx context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Verifies a krakenkey_domain once its TXT record is published. Create retries until KrakenKey finds " +
			"the record or the create timeout runs out. If KrakenKey's daily re-check later finds the record gone, the " +
			"domain becomes unverified and the next plan recreates this resource. Destroying it only removes it from state.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Same as domain_id.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"domain_id": schema.StringAttribute{
				Description:   "ID of the krakenkey_domain to verify. Changing it forces a new resource.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
			},
			"verified": schema.BoolAttribute{
				Description: "Always true while the resource exists.",
				Computed:    true,
			},
			"timeouts": timeouts.Attributes(ctx, timeouts.Opts{Create: true}),
		},
	}
}

func (r *domainVerificationResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	data, ok := req.ProviderData.(*providerData)
	if !ok {
		resp.Diagnostics.AddError("Unexpected provider data", fmt.Sprintf("got %T", req.ProviderData))
		return
	}
	r.data = data
}

func (r *domainVerificationResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan domainVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}
	timeout, diags := plan.Timeouts.Create(ctx, defaultVerifyTimeout)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	id := plan.DomainID.ValueString()
	if err := r.waitForVerified(ctx, id); err != nil {
		resp.Diagnostics.AddError("Domain "+id+" was not verified", err.Error())
		return
	}
	plan.ID = plan.DomainID
	plan.Verified = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// waitForVerified calls verify until it passes, a permanent error comes back,
// or ctx ends. Verify counts against the hourly rate limit for expensive
// operations (5 an hour on the Free plan), so the wait between attempts starts
// at 3 poll intervals (30s) and doubles up to 24 (4m): a 10 minute timeout
// makes at most 5 verify calls. Each attempt reads the domain first, which is
// cheap, and stops without calling verify if it is already verified.
func (r *domainVerificationResource) waitForVerified(ctx context.Context, id string) error {
	c := r.data.client
	wait, maxWait := 3*r.data.pollInterval, 24*r.data.pollInterval
	for {
		d, err := c.GetDomain(ctx, id)
		if err != nil {
			return err
		}
		if d.IsVerified {
			return nil
		}
		v, err := c.VerifyDomain(ctx, id)
		if err == nil && v.IsVerified {
			return nil
		}
		if err == nil {
			err = errors.New("verify returned the domain still unverified")
		} else if !domainVerifyRetryable(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("KrakenKey could not find the TXT record %s = %q before the create timeout ran out. "+
				"Check that it is published and visible from public DNS. Last error: %w", d.Hostname, d.VerificationCode, err)
		case <-time.After(wait):
		}
		wait = min(2*wait, maxWait)
	}
}

// domainVerifyRetryable reports whether a verify error may clear on its own.
// The API returns 400 when the TXT record is missing or the DNS lookup fails,
// which is normal while the record propagates. Gateway errors are retried too.
// Anything else (401, 403, 404, 429) will not change by waiting.
func domainVerifyRetryable(err error) bool {
	var apiErr *client.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.StatusCode {
	case http.StatusBadRequest, http.StatusBadGateway, http.StatusServiceUnavailable, http.StatusGatewayTimeout:
		return true
	}
	return false
}

func (r *domainVerificationResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state domainVerificationModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	d, err := r.data.client.GetDomain(ctx, state.DomainID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError("Could not read domain "+state.DomainID.ValueString(), err.Error())
		return
	}
	// An unverified domain (for example after the daily re-check failed)
	// drops out of state, so the next plan verifies it again.
	if !d.IsVerified {
		resp.State.RemoveResource(ctx)
		return
	}
	state.ID = types.StringValue(d.ID)
	state.Verified = types.BoolValue(true)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

// Update is never called with a change, since domain_id forces replacement
// and timeouts only matter on create.
func (r *domainVerificationResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state domainVerificationModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	plan.ID, plan.Verified = state.ID, state.Verified
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

// Delete only forgets the verification. The API has no way to unverify a domain.
func (r *domainVerificationResource) Delete(context.Context, resource.DeleteRequest, *resource.DeleteResponse) {
}

func (r *domainVerificationResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	if !uuidPattern.MatchString(req.ID) {
		resp.Diagnostics.AddError("Invalid import ID", "Import a domain verification by the domain's UUID.")
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), req.ID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("domain_id"), req.ID)...)
}
