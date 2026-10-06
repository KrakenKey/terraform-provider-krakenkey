package provider

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/setdefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/krakenkey/terraform-provider-krakenkey/internal/client"
)

var (
	_ resource.Resource                = (*alertChannelResource)(nil)
	_ resource.ResourceWithConfigure   = (*alertChannelResource)(nil)
	_ resource.ResourceWithImportState = (*alertChannelResource)(nil)
	_ resource.ResourceWithModifyPlan  = (*alertChannelResource)(nil)
)

// alertChannelMaxName is the API's limit on channel names.
const alertChannelMaxName = 100

func NewAlertChannelResource() resource.Resource {
	return &alertChannelResource{}
}

type alertChannelResource struct {
	data *providerData
}

type alertChannelModel struct {
	ID            types.String `tfsdk:"id"`
	Type          types.String `tfsdk:"type"`
	Name          types.String `tfsdk:"name"`
	URLWO         types.String `tfsdk:"url_wo"`
	URLWOVersion  types.Int64  `tfsdk:"url_wo_version"`
	Events        types.Set    `tfsdk:"events"`
	Enabled       types.Bool   `tfsdk:"enabled"`
	URLMasked     types.String `tfsdk:"url_masked"`
	SigningSecret types.String `tfsdk:"signing_secret"`
}

func (r *alertChannelResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_alert_channel"
}

func (r *alertChannelResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	defaultEvents := make([]attr.Value, len(client.DefaultAlertEvents))
	for i, e := range client.DefaultAlertEvents {
		defaultEvents[i] = types.StringValue(e)
	}

	resp.Schema = schema.Schema{
		Description: "A Slack, Microsoft Teams or signed webhook channel that receives KrakenKey alerts for every " +
			"certificate, domain and endpoint on the account. The destination URL is a write-only argument and " +
			"needs Terraform 1.11 or later. API keys limited to specific domains or certificates cannot manage channels.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Description:   "Channel UUID.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"type": schema.StringAttribute{
				Description:   "Channel type: " + strings.Join(client.AlertChannelTypes, ", ") + ". Changing it creates a new channel.",
				Required:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
				Validators:    []validator.String{alertChannelOneOf{allowed: client.AlertChannelTypes}},
			},
			"name": schema.StringAttribute{
				Description: fmt.Sprintf("Display name, up to %d characters.", alertChannelMaxName),
				Required:    true,
				Validators:  []validator.String{alertChannelNameValidator{}},
			},
			"url_wo": schema.StringAttribute{
				Description: "Destination URL. Slack: an incoming webhook (https://hooks.slack.com/services/...). " +
					"Teams: a Workflows webhook URL. Webhook: any https URL that resolves to public addresses. " +
					"The URL is a credential, so it is write-only and never stored in state or plan. It is sent on " +
					"create and whenever url_wo_version changes; changing only the URL does nothing.",
				Required:  true,
				WriteOnly: true,
				Sensitive: true,
			},
			"url_wo_version": schema.Int64Attribute{
				Description: "Change this (for example, increment it) to send url_wo to KrakenKey again.",
				Required:    true,
			},
			"events": schema.SetAttribute{
				Description: "Events to send: " + strings.Join(client.AlertEvents, ", ") + ". Defaults to " +
					strings.Join(client.DefaultAlertEvents, ", ") + ". An empty set subscribes to nothing.",
				ElementType: types.StringType,
				Optional:    true,
				Computed:    true,
				Default:     setdefault.StaticValue(types.SetValueMust(types.StringType, defaultEvents)),
				Validators:  []validator.Set{alertChannelEventsValidator{}},
			},
			"enabled": schema.BoolAttribute{
				Description: "Whether alerts are sent. Default true.",
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
			},
			"url_masked": schema.StringAttribute{
				Description:   "The destination as the API shows it: scheme, host and the last 4 characters.",
				Computed:      true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"signing_secret": schema.StringAttribute{
				Description: "Webhook channels only: the secret KrakenKey signs each delivery with (X-KrakenKey-Signature). " +
					"The API returns it once, on create, so it is kept in state; protect the state accordingly. " +
					"It is null for Slack and Teams channels and for imported channels. Rotating it in the dashboard " +
					"does not update this value.",
				Computed:      true,
				Sensitive:     true,
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

func (r *alertChannelResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
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

// ModifyPlan marks url_masked unknown when a new URL is about to be sent, so
// the plan shows it changing.
func (r *alertChannelResource) ModifyPlan(ctx context.Context, req resource.ModifyPlanRequest, resp *resource.ModifyPlanResponse) {
	if req.State.Raw.IsNull() || req.Plan.Raw.IsNull() {
		return
	}
	var plan, state alertChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if !plan.URLWOVersion.Equal(state.URLWOVersion) {
		resp.Diagnostics.Append(resp.Plan.SetAttribute(ctx, path.Root("url_masked"), types.StringUnknown())...)
	}
}

func (r *alertChannelResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan alertChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	url, diags := alertChannelConfigURL(ctx, req.Config.GetAttribute)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	var events []string
	resp.Diagnostics.Append(plan.Events.ElementsAs(ctx, &events, false)...)
	if resp.Diagnostics.HasError() {
		return
	}

	ch, err := r.data.client.CreateAlertChannel(ctx, client.CreateAlertChannelRequest{
		Type:    plan.Type.ValueString(),
		Name:    plan.Name.ValueString(),
		URL:     url,
		Events:  events,
		Enabled: plan.Enabled.ValueBool(),
	})
	if err != nil {
		resp.Diagnostics.AddError("Could not create the alert channel", err.Error())
		return
	}
	// The signing secret is only in this response; the API never returns it again.
	if ch.Secret != "" {
		plan.SigningSecret = types.StringValue(ch.Secret)
	} else {
		plan.SigningSecret = types.StringNull()
	}
	resp.Diagnostics.Append(alertChannelApply(ctx, ch, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertChannelResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state alertChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	ch, err := r.data.client.GetAlertChannel(ctx, state.ID.ValueString())
	if client.IsNotFound(err) {
		resp.State.RemoveResource(ctx)
		return
	}
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not read alert channel %s", state.ID.ValueString()), err.Error())
		return
	}
	resp.Diagnostics.Append(alertChannelApply(ctx, ch, &state)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, state)...)
}

func (r *alertChannelResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state alertChannelModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	var events []string
	resp.Diagnostics.Append(plan.Events.ElementsAs(ctx, &events, false)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if events == nil {
		events = []string{}
	}
	name, enabled := plan.Name.ValueString(), plan.Enabled.ValueBool()
	in := client.UpdateAlertChannelRequest{Name: &name, Events: &events, Enabled: &enabled}

	// The URL is only sent when url_wo_version changes. Write-only values are
	// never in plan or state, so it has to come from the configuration.
	if !plan.URLWOVersion.Equal(state.URLWOVersion) {
		url, diags := alertChannelConfigURL(ctx, req.Config.GetAttribute)
		resp.Diagnostics.Append(diags...)
		if resp.Diagnostics.HasError() {
			return
		}
		in.URL = &url
	}

	ch, err := r.data.client.UpdateAlertChannel(ctx, state.ID.ValueString(), in)
	if err != nil {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not update alert channel %s", state.ID.ValueString()), err.Error())
		return
	}
	plan.ID = state.ID
	plan.SigningSecret = state.SigningSecret
	resp.Diagnostics.Append(alertChannelApply(ctx, ch, &plan)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, plan)...)
}

func (r *alertChannelResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state alertChannelModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if err := r.data.client.DeleteAlertChannel(ctx, state.ID.ValueString()); err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError(fmt.Sprintf("Could not delete alert channel %s", state.ID.ValueString()), err.Error())
	}
}

// ImportState takes the channel UUID. url_wo can't be imported (the API never
// returns it), so the first apply after an import sends the configured URL
// once, because state has no url_wo_version yet. signing_secret stays null.
func (r *alertChannelResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// alertChannelConfigURL reads url_wo from the configuration, the only place a
// write-only value is available.
func alertChannelConfigURL(ctx context.Context, get func(context.Context, path.Path, any) diag.Diagnostics) (string, diag.Diagnostics) {
	var url types.String
	diags := get(ctx, path.Root("url_wo"), &url)
	if diags.HasError() {
		return "", diags
	}
	if url.IsNull() || url.IsUnknown() {
		diags.AddAttributeError(path.Root("url_wo"), "Missing channel URL",
			"url_wo must be known when the channel is created or url_wo_version changes.")
	}
	return url.ValueString(), diags
}

// alertChannelApply copies the API's view of the channel into m. url_wo is
// always null in state.
func alertChannelApply(ctx context.Context, ch *client.AlertChannel, m *alertChannelModel) diag.Diagnostics {
	m.ID = types.StringValue(ch.ID)
	m.Type = types.StringValue(ch.Type)
	m.Name = types.StringValue(ch.Name)
	m.Enabled = types.BoolValue(ch.Enabled)
	m.URLMasked = types.StringValue(ch.URLMasked)
	m.URLWO = types.StringNull()
	if m.SigningSecret.IsUnknown() {
		m.SigningSecret = types.StringNull()
	}
	events := ch.Events
	if events == nil {
		events = []string{}
	}
	var diags diag.Diagnostics
	m.Events, diags = types.SetValueFrom(ctx, types.StringType, events)
	return diags
}

// alertChannelOneOf checks a string against a fixed list of API values.
type alertChannelOneOf struct {
	allowed []string
}

func (v alertChannelOneOf) Description(_ context.Context) string {
	return "must be one of: " + strings.Join(v.allowed, ", ")
}

func (v alertChannelOneOf) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v alertChannelOneOf) ValidateString(_ context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	if got := req.ConfigValue.ValueString(); !slices.Contains(v.allowed, got) {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid value",
			fmt.Sprintf("%q is not supported; use one of: %s.", got, strings.Join(v.allowed, ", ")))
	}
}

// alertChannelEventsValidator checks every element of events against the
// API's event list.
type alertChannelEventsValidator struct{}

func (v alertChannelEventsValidator) Description(_ context.Context) string {
	return "each element must be one of: " + strings.Join(client.AlertEvents, ", ")
}

func (v alertChannelEventsValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v alertChannelEventsValidator) ValidateSet(_ context.Context, req validator.SetRequest, resp *validator.SetResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	for _, el := range req.ConfigValue.Elements() {
		s, ok := el.(types.String)
		if !ok || s.IsNull() || s.IsUnknown() {
			continue
		}
		if !slices.Contains(client.AlertEvents, s.ValueString()) {
			resp.Diagnostics.AddAttributeError(req.Path, "Invalid alert event",
				fmt.Sprintf("%q is not an alert event; use one of: %s.", s.ValueString(), strings.Join(client.AlertEvents, ", ")))
		}
	}
}

// alertChannelNameValidator matches the API's name rules. The API trims
// surrounding whitespace, which would leave state different from the
// configuration, so that is rejected here instead.
type alertChannelNameValidator struct{}

func (v alertChannelNameValidator) Description(_ context.Context) string {
	return fmt.Sprintf("must be 1 to %d characters with no leading or trailing whitespace", alertChannelMaxName)
}

func (v alertChannelNameValidator) MarkdownDescription(ctx context.Context) string {
	return v.Description(ctx)
}

func (v alertChannelNameValidator) ValidateString(ctx context.Context, req validator.StringRequest, resp *validator.StringResponse) {
	if req.ConfigValue.IsNull() || req.ConfigValue.IsUnknown() {
		return
	}
	name := req.ConfigValue.ValueString()
	if name == "" || strings.TrimSpace(name) != name || utf8.RuneCountInString(name) > alertChannelMaxName {
		resp.Diagnostics.AddAttributeError(req.Path, "Invalid channel name", "The name "+v.Description(ctx)+".")
	}
}
