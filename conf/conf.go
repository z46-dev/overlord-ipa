package conf

import "github.com/z46-dev/goconf"

type Config struct {
	Database DatabaseConfig `toml:"database"`
	FreeIPA  FreeIPAConfig  `toml:"freeipa"`
	Server   ServerConfig   `toml:"server"`
	Auth     AuthConfig     `toml:"auth"`
	Data     DataConfig     `toml:"data"`
}

type DatabaseConfig struct {
	File string `toml:"file" default:"overlord-ipa.db" validate:"required,filepath"`
}

type FreeIPAConfig struct {
	Server             string `toml:"server" default:"" validate:"required,fqdn"`
	Port               uint   `toml:"port" default:"636" validate:"required,port"`
	UseTLS             bool   `toml:"use_tls" default:"true"`
	StartTLS           bool   `toml:"start_tls" default:"false"`
	Realm              string `toml:"realm" default:"" validate:"required"`
	BaseDN             string `toml:"base_dn" default:"" validate:"required"`
	HostBaseDN         string `toml:"host_base_dn" default:"" validate:"required"`
	HostGroupBaseDN    string `toml:"host_group_base_dn" default:"" validate:"required"`
	BindDN             string `toml:"bind_dn" default:"" validate:"required"`
	BindPassword       string `toml:"bind_password" default:"" validate:"omitempty"`
	ConnectTimeout     string `toml:"connect_timeout" default:"10s" validate:"required"`
	RequestTimeout     string `toml:"request_timeout" default:"30s" validate:"required"`
	InsecureSkipVerify bool   `toml:"insecure_skip_verify" default:"false"`
	CACertFile         string `toml:"ca_cert_file" default:"" validate:"omitempty,filepath"`
}

type ServerConfig struct {
	Host          string `toml:"host" default:"127.0.0.1" validate:"required,ip|hostname"`
	Port          uint   `toml:"port" default:"8080" validate:"required,port"`
	RedirectPorts []uint `toml:"redirect_ports" validate:"omitempty,dive,port"`
	TLSCertFile   string `toml:"tls_cert_file" validate:"omitempty,filepath"`
	TLSKeyFile    string `toml:"tls_key_file" validate:"omitempty,filepath"`
	StaticDir     string `toml:"static_dir" default:"client/static" validate:"required"`
	SPAFallback   bool   `toml:"spa_fallback" default:"true"`
}

type AuthConfig struct {
	ViewerGroups  []string `toml:"viewer_groups" validate:"omitempty,dive,required"`
	EditorGroups  []string `toml:"editor_groups" validate:"omitempty,dive,required"`
	SessionCookie string   `toml:"session_cookie" default:"overlord_ipa_session" validate:"required"`
	SessionTTL    string   `toml:"session_ttl" default:"8h" validate:"required"`
}

type DataConfig struct {
	Directory string `toml:"directory" default:"data" validate:"required"`
}

var Conf Config

// Init loads the application configuration from disk.
func Init() (err error) {
	if Conf, err = goconf.LoadConfig[Config]("config.toml", goconf.WithIndentSpaces(4), goconf.WithNewFileBehavior(goconf.NewFileBehaviorCreateAndTry)); err != nil {
		return
	}

	return
}
