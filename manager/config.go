// Copyright 2015 Prometheus Team
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package manager

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/prometheus/common/model"
	"gopkg.in/yaml.v2"
)

var (
	DefaultHipchatConfig = HipchatConfig{
		Color:         "purple",
		ColorResolved: "green",
		MessageFormat: HipchatFormatHTML,
	}

	DefaultSlackConfig = SlackConfig{
		Color:         "warning",
		ColorResolved: "good",
	}
)

// Load parses the YAML input s into a Config.
func Load(s string) (*Config, error) {
	cfg := &Config{}
	err := yaml.Unmarshal([]byte(s), cfg)
	if err != nil {
		return nil, err
	}
	cfg.original = s
	return cfg, nil
}

// LoadFile parses the given YAML file into a Config.
func LoadFile(filename string) (*Config, error) {
	content, err := ioutil.ReadFile(filename)
	if err != nil {
		return nil, err
	}
	return Load(string(content))
}

// Config is the top-level configuration for Alertmanager's config files.
type Config struct {
	Routes              Routes                `yaml:"routes,omitempty"`
	InhibitRules        []*InhibitRule        `yaml:"inhibit_rules,omitempty"`
	NotificationConfigs []*NotificationConfig `yaml:"notification_configs,omitempty"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`

	// original is the input from which the config was parsed.
	original string
}

func checkOverflow(m map[string]interface{}, ctx string) error {
	if len(m) > 0 {
		var keys []string
		for k := range m {
			keys = append(keys, k)
		}
		return fmt.Errorf("unknown fields in %s: %s", ctx, strings.Join(keys, ", "))
	}
	return nil
}

func (c Config) String() string {
	if c.original != "" {
		return c.original
	}
	b, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("<error creating config string: %s>", err)
	}
	return string(b)
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *Config) UnmarshalYAML(unmarshal func(interface{}) error) error {
	// We want to set c to the defaults and then overwrite it with the input.
	// To make unmarshal fill the plain data struct rather than calling UnmarshalYAML
	// again, we have to hide it using a type indirection.
	type plain Config
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}

	names := map[string]struct{}{}

	for _, nc := range c.NotificationConfigs {
		if _, ok := names[nc.Name]; ok {
			return fmt.Errorf("notification config name %q is not unique", nc.Name)
		}
		names[nc.Name] = struct{}{}
	}
	return checkOverflow(c.XXX, "config")
}

// An InhibitRule specifies that a class of (source) alerts should inhibit
// notifications for another class of (target) alerts if all specified matching
// labels are equal between the two alerts. This may be used to inhibit alerts
// from sending notifications if their meaning is logically a subset of a
// higher-level alert.
type InhibitRule struct {
	// The set of Filters which define the group of source alerts (which inhibit
	// the target alerts).
	SourceMatchers Matchers

	// The set of Filters which define the group of target alerts (which are
	// inhibited by the source alerts).
	TargetMatchers Matchers

	// A set of label names whose label values need to be identical in source and
	// target alerts in order for the inhibition to take effect.
	Equal model.LabelNames

	// How many seconds to wait for a corresponding inhibit source alert to
	// appear before sending any notifications for active target alerts.
	// TODO(julius): Not supported yet. Implement this!
	// optional int32 before_allowance = 4 [default = 0];
	// How many seconds to wait after a corresponding inhibit source alert
	// disappears before sending any notifications for active target alerts.
	// TODO(julius): Not supported yet. Implement this!
	// optional int32 after_allowance = 5 [default = 0];
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (r *InhibitRule) UnmarshalYAML(unmarshal func(interface{}) error) error {
	v := struct {
		SourceMatch   map[string]string `yaml:"source_match"`
		SourceMatchRE map[string]string `yaml:"source_match_re"`
		TargetMatch   map[string]string `yaml:"target_match"`
		TargetMatchRE map[string]string `yaml:"target_match_re"`
		Equal         model.LabelNames  `yaml:"equal"`

		// Catches all undefined fields and must be empty after parsing.
		XXX map[string]interface{} `yaml:",inline"`
	}{}
	if err := unmarshal(&v); err != nil {
		return err
	}

	for k, val := range v.SourceMatch {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)
		r.SourceMatchers = append(r.SourceMatchers, NewMatcher(ln, val))
	}

	for k, val := range v.SourceMatchRE {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)

		m, err := NewRegexMatcher(ln, val)
		if err != nil {
			return err
		}
		r.SourceMatchers = append(r.SourceMatchers, m)
	}

	for k, val := range v.TargetMatch {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)
		r.TargetMatchers = append(r.TargetMatchers, NewMatcher(ln, val))
	}

	for k, val := range v.TargetMatchRE {
		if !model.LabelNameRE.MatchString(k) {
			fmt.Errorf("invalid label name %q", k)
		}
		ln := model.LabelName(k)

		m, err := NewRegexMatcher(ln, val)
		if err != nil {
			return err
		}
		r.TargetMatchers = append(r.TargetMatchers, m)
	}

	r.Equal = v.Equal

	return checkOverflow(v.XXX, "inhibit rule")
}

// Notification configuration definition.
type NotificationConfig struct {
	// Name of this NotificationConfig. Referenced from AggregationRule.
	Name string `yaml:"name"`

	// Notify when resolved.
	SendResolved bool `yaml:"send_resolved"`

	PagerdutyConfigs []*PagerdutyConfig `yaml:"pagerduty_configs"`
	EmailConfigs     []*EmailConfig     `yaml:"email_configs"`
	PushoverConfigs  []*PushoverConfig  `yaml:"pushover_configs"`
	HipchatConfigs   []*HipchatConfig   `yaml:"hipchat_configs"`
	SlackConfigs     []*SlackConfig     `yaml:"slack_config"`
	FlowdockConfigs  []*FlowdockConfig  `yaml:"flowdock_config"`
	WebhookConfigs   []*WebhookConfig   `yaml:"webhook_config"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *NotificationConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain NotificationConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Name == "" {
		return fmt.Errorf("missing name in notification config")
	}
	return checkOverflow(c.XXX, "notification config")
}

// Configuration for notification via PagerDuty.
type PagerdutyConfig struct {
	// PagerDuty service key, see:
	// http://developer.pagerduty.com/documentation/integration/events
	ServiceKey string `yaml:"service_key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PagerdutyConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PagerdutyConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.ServiceKey == "" {
		return fmt.Errorf("missing service key in PagerDuty config")
	}
	return checkOverflow(c.XXX, "pagerduty config")
}

// Configuration for notification via mail.
type EmailConfig struct {
	// Email address to notify.
	Email string `yaml:"email"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *EmailConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain EmailConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Email == "" {
		return fmt.Errorf("missing email address in email config")
	}
	return checkOverflow(c.XXX, "email config")
}

// Configuration for notification via pushover.net.
type PushoverConfig struct {
	// Pushover token.
	Token string `yaml:"token"`

	// Pushover user_key.
	UserKey string `yaml:"user_key"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *PushoverConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain PushoverConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.Token == "" {
		return fmt.Errorf("missing token in Pushover config")
	}
	if c.UserKey == "" {
		return fmt.Errorf("missing user key in Pushover config")
	}
	return checkOverflow(c.XXX, "pushover config")
}

type HipchatFormat string

const (
	HipchatFormatHTML HipchatFormat = "html"
	HipchatFormatText HipchatFormat = "text"
)

// Configuration for notification via HipChat.
// https://www.hipchat.com/docs/apiv2/method/send_room_notification
type HipchatConfig struct {
	// HipChat auth token, (https://www.hipchat.com/docs/api/auth).
	AuthToken string `yaml:"auth_token"`

	// HipChat room id, (https://www.hipchat.com/rooms/ids).
	RoomID int `yaml:"room_id"`

	// Color of message when triggered.
	Color string `yaml:"color"`

	// Color of message when resolved.
	ColorResolved string `yaml:"color_resolved"`

	// Should this message notify or not.
	Notify bool `yaml:"notify"`

	// Prefix to be put in front of the message (useful for @mentions, etc.).
	Prefix string `yaml:"prefix"`

	// Format the message as "html" or "text".
	MessageFormat HipchatFormat `yaml:"message_format"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *HipchatConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultHipchatConfig
	type plain HipchatConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.AuthToken == "" {
		return fmt.Errorf("missing auth token in HipChat config")
	}
	if c.MessageFormat != HipchatFormatHTML && c.MessageFormat != HipchatFormatText {
		return fmt.Errorf("invalid message format %q", c.MessageFormat)
	}
	return checkOverflow(c.XXX, "hipchat config")
}

// Configuration for notification via Slack.
type SlackConfig struct {
	// Slack webhook URL, (https://api.slack.com/incoming-webhooks).
	WebhookURL string `yaml:"webhook_url"`

	// Slack channel override, (like #other-channel or @username).
	Channel string `yaml:"channel"`

	// Color of message when triggered.
	Color string `yaml:"color"`

	// Color of message when resolved.
	ColorResolved string `yaml:"color_resolved"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *SlackConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	*c = DefaultSlackConfig
	type plain SlackConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.WebhookURL == "" {
		return fmt.Errorf("missing webhook URL in Slack config")
	}
	if c.Channel == "" {
		return fmt.Errorf("missing channel in Slack config")
	}
	return checkOverflow(c.XXX, "slack config")
}

// Configuration for notification via Flowdock.
type FlowdockConfig struct {
	// Flowdock flow API token.
	APIToken string `yaml:"api_token"`

	// Flowdock from_address.
	FromAddress string `yaml:"from_address"`

	// Flowdock flow tags.
	Tags []string `yaml:"tags"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *FlowdockConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain FlowdockConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.APIToken == "" {
		return fmt.Errorf("missing API token in Flowdock config")
	}
	if c.FromAddress == "" {
		return fmt.Errorf("missing from address in Flowdock config")
	}
	return checkOverflow(c.XXX, "flowdock config")
}

// Configuration for notification via generic webhook.
type WebhookConfig struct {
	// URL to send POST request to.
	URL string `yaml:"url"`

	// Catches all undefined fields and must be empty after parsing.
	XXX map[string]interface{} `yaml:",inline"`
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (c *WebhookConfig) UnmarshalYAML(unmarshal func(interface{}) error) error {
	type plain WebhookConfig
	if err := unmarshal((*plain)(c)); err != nil {
		return err
	}
	if c.URL == "" {
		return fmt.Errorf("missing URL in webhook config")
	}
	return checkOverflow(c.XXX, "slack config")
}

// Regexp encapsulates a regexp.Regexp and makes it YAML marshallable.
type Regexp struct {
	regexp.Regexp
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (re *Regexp) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	regex, err := regexp.Compile(s)
	if err != nil {
		return err
	}
	re.Regexp = *regex
	return nil
}

// MarshalYAML implements the yaml.Marshaler interface.
func (re *Regexp) MarshalYAML() (interface{}, error) {
	if re != nil {
		return re.String(), nil
	}
	return nil, nil
}