package dnsresolver

import (
	"bytes"
	"html/template"
	gonet "net"
	"strings"

	bosherr "github.com/cloudfoundry/bosh-utils/errors"
	boshsys "github.com/cloudfoundry/bosh-utils/system"
)

const (
	systemdResolvedTemplate = `# Generated by bosh-agent
[Resolve]
DNS={{ range . }}{{ . }}{{ " " -}}
{{ end }}`
)

type systemdResolver struct {
	fs        boshsys.FileSystem
	cmdRunner boshsys.CmdRunner
}

func NewSystemdResolver(
	fs boshsys.FileSystem,
	cmdRunner boshsys.CmdRunner,
) DNSResolver {
	return &systemdResolver{
		fs:        fs,
		cmdRunner: cmdRunner,
	}
}

func (d *systemdResolver) Validate(dnsServers []string) error {
	resolvConfContents, err := d.fs.ReadFileString("/etc/systemd/resolved.conf.d/10-bosh.conf")
	if err != nil {
		return bosherr.WrapError(err, "Reading /etc/systemd/resolved.conf.d/10-bosh.conf")
	}

	for _, dnsServer := range dnsServers {
		if strings.Contains(resolvConfContents, dnsServer) {
			return nil
		}

		canonicalIP := gonet.ParseIP(dnsServer)

		if canonicalIP != nil {
			if strings.Contains(resolvConfContents, canonicalIP.String()) {
				return nil
			}
		}
	}
	return bosherr.WrapError(err, "None of the DNS servers that were specified in the manifest were found in /etc/resolv.conf.")
}

func (d *systemdResolver) SetupDNS(dnsServers []string) error {
	// mkdir /etc/systemd/resolved.conf.d/
	// create /etc/systemd/resolved.conf.d/10-bosh.conf
	// contents (if your dns Servers are 1.1.1.1 and 8.8.8.8):
	// DNS=8.8.8.8 1.1.1.1
	if len(dnsServers) == 0 {
		return nil
	}
	var buffer bytes.Buffer
	t := template.Must(template.New("resolv-conf").Parse(systemdResolvedTemplate))
	err := t.Execute(&buffer, dnsServers)
	if err != nil {
		return bosherr.WrapError(err, "Generating DNS config from template")
	}

	// Create the directory; it doesn't exist on a vanilla Noble install
	dirPath := "/etc/systemd/resolved.conf.d/"
	err = d.fs.MkdirAll(dirPath, 0755)
	if err != nil {
		return bosherr.WrapError(err, "Creating directory "+dirPath)
	}
	systemdResolvedPath := dirPath + "10-bosh.conf"
	err = d.fs.WriteFile(systemdResolvedPath, buffer.Bytes())
	if err != nil {
		return bosherr.WrapError(err, "Writing to "+systemdResolvedPath)
	}
	_, _, _, err = d.cmdRunner.RunCommand("systemctl", "restart", "systemd-resolved")
	if err != nil {
		return bosherr.WrapError(err, "restarting systemd-resolved")
	}

	return nil
}
