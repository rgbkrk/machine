package openstack

import (
	"fmt"
	"io/ioutil"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/docker/utils"
	"github.com/docker/machine/drivers"
	"github.com/docker/machine/ssh"
	"github.com/docker/machine/state"
)

type Driver struct {
	AuthUrl             string
	Username            string
	Password            string
	TenantName          string
	TenantId            string
	Region              string
	EndpointType        string
	MachineName         string
	MachineId           string
	FlavorName          string
	FlavorId            string
	ImageName           string
	ImageId             string
	KeyPairName         string
	NetworkName         string
	NetworkId           string
	SecurityGroups      []string
	FloatingIpPool      string
	FloatingIpPoolId    string
	SSHUser             string
	SSHPort             int
	Ip                  string
	EnableDockerInstall bool
	storePath           string
	client              Client
}

type CreateFlags struct {
	AuthUrl        *string
	Username       *string
	Password       *string
	TenantName     *string
	TenantId       *string
	Region         *string
	EndpointType   *string
	FlavorName     *string
	FlavorId       *string
	ImageName      *string
	ImageId        *string
	NetworkName    *string
	NetworkId      *string
	SecurityGroups *string
	FloatingIpPool *string
	SSHUser        *string
	SSHPort        *int
}

func init() {
	drivers.Register("openstack", &drivers.RegisteredDriver{
		New:            NewDriver,
		GetCreateFlags: GetCreateFlags,
	})
}

func GetCreateFlags() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			EnvVar: "OS_AUTH_URL",
			Name:   "openstack-auth-url",
			Usage:  "OpenStack authentication URL",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_USERNAME",
			Name:   "openstack-username",
			Usage:  "OpenStack username",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_PASSWORD",
			Name:   "openstack-password",
			Usage:  "OpenStack password",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_TENANT_NAME",
			Name:   "openstack-tenant-name",
			Usage:  "OpenStack tenant name",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_TENANT_ID",
			Name:   "openstack-tenant-id",
			Usage:  "OpenStack tenant id",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_REGION_NAME",
			Name:   "openstack-region",
			Usage:  "OpenStack region name",
			Value:  "",
		},
		cli.StringFlag{
			EnvVar: "OS_ENDPOINT_TYPE",
			Name:   "openstack-endpoint-type",
			Usage:  "OpenStack endpoint type (adminURL, internalURL or publicURL)",
			Value:  "",
		},
		cli.StringFlag{
			Name:  "openstack-flavor-id",
			Usage: "OpenStack flavor id to use for the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-flavor-name",
			Usage: "OpenStack flavor name to use for the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-image-id",
			Usage: "OpenStack image id to use for the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-image-name",
			Usage: "OpenStack image name to use for the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-net-id",
			Usage: "OpenStack image name to use for the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-net-name",
			Usage: "OpenStack network name the machine will be connected on",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-sec-groups",
			Usage: "OpenStack comma separated security groups for the machine",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-floatingip-pool",
			Usage: "OpenStack floating IP pool to get an IP from to assign to the instance",
			Value: "",
		},
		cli.StringFlag{
			Name:  "openstack-ssh-user",
			Usage: "OpenStack SSH user",
			Value: "root",
		},
		cli.IntFlag{
			Name:  "openstack-ssh-port",
			Usage: "OpenStack SSH port",
			Value: 22,
		},
		// Using a StringFlag rather than a BoolFlag because
		// the BoolFlag default value is always false
		cli.StringFlag{
			Name:  "openstack-docker-install",
			Usage: "Set if docker have to be installed on the machine",
			Value: "true",
		},
	}
}

func NewDriver(storePath string) (drivers.Driver, error) {
	log.WithFields(log.Fields{
		"storePath": storePath,
	}).Debug("Instantiating OpenStack driver...")

	return NewDerivedDriver(storePath, &GenericClient{})
}

func NewDerivedDriver(storePath string, client Client) (*Driver, error) {
	return &Driver{
		storePath: storePath,
		client:    client,
	}, nil
}

func (d *Driver) DriverName() string {
	return "openstack"
}

func (d *Driver) SetConfigFromFlags(flags drivers.DriverOptions) error {
	d.AuthUrl = flags.String("openstack-auth-url")
	d.Username = flags.String("openstack-username")
	d.Password = flags.String("openstack-password")
	d.TenantName = flags.String("openstack-tenant-name")
	d.TenantId = flags.String("openstack-tenant-id")
	d.Region = flags.String("openstack-region")
	d.EndpointType = flags.String("openstack-endpoint-type")
	d.FlavorId = flags.String("openstack-flavor-id")
	d.FlavorName = flags.String("openstack-flavor-name")
	d.ImageId = flags.String("openstack-image-id")
	d.ImageName = flags.String("openstack-image-name")
	d.NetworkId = flags.String("openstack-net-id")
	d.NetworkName = flags.String("openstack-net-name")
	if flags.String("openstack-sec-groups") != "" {
		d.SecurityGroups = strings.Split(flags.String("openstack-sec-groups"), ",")
	}
	d.FloatingIpPool = flags.String("openstack-floatingip-pool")
	d.SSHUser = flags.String("openstack-ssh-user")
	d.SSHPort = flags.Int("openstack-ssh-port")

	installDocker, err := strconv.ParseBool(flags.String("openstack-docker-install"))
	if err != nil {
		return err
	}
	d.EnableDockerInstall = installDocker
	return nil
}

func (d *Driver) GetURL() (string, error) {
	ip, err := d.GetIP()
	if err != nil {
		return "", err
	}
	if ip == "" {
		return "", nil
	}
	return fmt.Sprintf("tcp://%s:2376", ip), nil
}

func (d *Driver) GetIP() (string, error) {
	if d.Ip != "" {
		return d.Ip, nil
	}

	log.WithField("MachineId", d.MachineId).Debug("Looking for the IP address...")

	if err := d.initCompute(); err != nil {
		return "", err
	}

	addressType := Fixed
	if d.FloatingIpPool != "" {
		addressType = Floating
	}

	// Looking for the IP address in a retry loop to deal with OpenStack latency
	for retryCount := 0; retryCount < 200; retryCount++ {
		addresses, err := d.client.GetInstanceIpAddresses(d)
		if err != nil {
			return "", err
		}
		for _, a := range addresses {
			if a.AddressType == addressType {
				return a.Address, nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return "", fmt.Errorf("No IP found for the machine")
}

func (d *Driver) GetState() (state.State, error) {
	log.WithField("MachineId", d.MachineId).Debug("Get status for OpenStack instance...")
	if err := d.initCompute(); err != nil {
		return state.None, err
	}

	s, err := d.client.GetInstanceState(d)
	if err != nil {
		return state.None, err
	}

	log.WithFields(log.Fields{
		"MachineId": d.MachineId,
		"State":     s,
	}).Debug("State for OpenStack instance")

	switch s {
	case "ACTIVE":
		return state.Running, nil
	case "PAUSED":
		return state.Paused, nil
	case "SUSPENDED":
		return state.Saved, nil
	case "SHUTOFF":
		return state.Stopped, nil
	case "BUILDING":
		return state.Starting, nil
	case "ERROR":
		return state.Error, nil
	}
	return state.None, nil
}

func (d *Driver) Create() error {
	d.setMachineNameIfNotSet()
	d.KeyPairName = d.MachineName

	if err := d.resolveIds(); err != nil {
		return err
	}
	if err := d.createSSHKey(); err != nil {
		return err
	}
	if err := d.createMachine(); err != nil {
		return err
	}
	if err := d.waitForInstanceActive(); err != nil {
		return err
	}
	if d.FloatingIpPool != "" {
		if err := d.assignFloatingIp(); err != nil {
			return err
		}
	}
	if err := d.lookForIpAddress(); err != nil {
		return err
	}
	if err := d.waitForSSHServer(); err != nil {
		return err
	}
	if d.EnableDockerInstall {
		if err := d.installDocker(); err != nil {
			return err
		}
	}
	return nil
}

func (d *Driver) Start() error {
	log.WithField("MachineId", d.MachineId).Info("Starting OpenStack instance...")
	if err := d.initCompute(); err != nil {
		return err
	}
	if err := d.client.StartInstance(d); err != nil {
		return err
	}
	return d.waitForInstanceToStart()
}

func (d *Driver) Stop() error {
	log.WithField("MachineId", d.MachineId).Info("Stopping OpenStack instance...")
	if err := d.initCompute(); err != nil {
		return err
	}
	if err := d.client.StopInstance(d); err != nil {
		return err
	}

	log.WithField("MachineId", d.MachineId).Info("Waiting for the OpenStack instance to stop...")
	if err := d.client.WaitForInstanceStatus(d, "SHUTOFF", 200); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Remove() error {
	log.WithField("MachineId", d.MachineId).Info("Deleting OpenStack instance...")
	if err := d.initCompute(); err != nil {
		return err
	}
	if err := d.client.DeleteInstance(d); err != nil {
		return err
	}
	log.WithField("Name", d.KeyPairName).Info("Deleting Key Pair...")
	if err := d.client.DeleteKeyPair(d, d.KeyPairName); err != nil {
		return err
	}
	return nil
}

func (d *Driver) Restart() error {
	log.WithField("MachineId", d.MachineId).Info("Restarting OpenStack instance...")
	if err := d.initCompute(); err != nil {
		return err
	}
	if err := d.client.RestartInstance(d); err != nil {
		return err
	}
	return d.waitForInstanceToStart()
}

func (d *Driver) Kill() error {
	return d.Stop()
}

func (d *Driver) Upgrade() error {
	return fmt.Errorf("Upgrate is currently not available for the OpenStack driver")
}

func (d *Driver) GetSSHCommand(args ...string) (*exec.Cmd, error) {
	ip, err := d.GetIP()
	if err != nil {
		return nil, err
	}

	if len(args) != 0 && d.SSHUser != "root" {
		cmd := strings.Replace(strings.Join(args, " "), "'", "\\'", -1)
		args = []string{"sudo", "sh", "-c", fmt.Sprintf("'%s'", cmd)}
	}

	log.WithField("MachineId", d.MachineId).Debug("Command: %s", args)
	return ssh.GetSSHCommand(ip, d.SSHPort, d.SSHUser, d.sshKeyPath(), args...), nil
}

const (
	errorMandatoryEnvOrOption    string = "%s must be specified either using the environment variable %s or the CLI option %s"
	errorMandatoryOption         string = "%s must be specified using the CLI option %s"
	errorExclusiveOptions        string = "Either %s or %s must be specified, not both"
	errorMandatoryTenantNameOrId string = "Tenant id or name must be provided either using one of the environment variables OS_TENANT_ID and OS_TENANT_NAME or one of the CLI options --openstack-tenant-id and --openstack-tenant-name"
	errorWrongEndpointType       string = "Endpoint type must be 'publicURL', 'adminURL' or 'internalURL'"
	errorUnknownFlavorName       string = "Unable to find flavor named %s"
	errorUnknownImageName        string = "Unable to find image named %s"
	errorUnknownNetworkName      string = "Unable to find network named %s"
)

func (d *Driver) checkConfig() error {
	if d.AuthUrl == "" {
		return fmt.Errorf(errorMandatoryEnvOrOption, "Autentication URL", "OS_AUTH_URL", "--openstack-auth-url")
	}
	if d.Username == "" {
		return fmt.Errorf(errorMandatoryEnvOrOption, "Username", "OS_USERNAME", "--openstack-username")
	}
	if d.Password == "" {
		return fmt.Errorf(errorMandatoryEnvOrOption, "Password", "OS_PASSWORD", "--openstack-password")
	}
	if d.TenantName == "" && d.TenantId == "" {
		return fmt.Errorf(errorMandatoryTenantNameOrId)
	}

	if d.FlavorName == "" && d.FlavorId == "" {
		return fmt.Errorf(errorMandatoryOption, "Flavor name or Flavor id", "--openstack-flavor-name or --openstack-flavor-id")
	}
	if d.FlavorName != "" && d.FlavorId != "" {
		return fmt.Errorf(errorExclusiveOptions, "Flavor name", "Flavor id")
	}

	if d.ImageName == "" && d.ImageId == "" {
		return fmt.Errorf(errorMandatoryOption, "Image name or Image id", "--openstack-image-name or --openstack-image-id")
	}
	if d.ImageName != "" && d.ImageId != "" {
		return fmt.Errorf(errorExclusiveOptions, "Image name", "Image id")
	}

	if d.NetworkName != "" && d.NetworkId != "" {
		return fmt.Errorf(errorExclusiveOptions, "Network name", "Network id")
	}
	if d.EndpointType != "" && (d.EndpointType != "publicURL" && d.EndpointType != "adminURL" && d.EndpointType != "internalURL") {
		return fmt.Errorf(errorWrongEndpointType)
	}
	return nil
}

func (d *Driver) resolveIds() error {
	if d.NetworkName != "" {
		if err := d.initNetwork(); err != nil {
			return err
		}

		networkId, err := d.client.GetNetworkId(d)

		if err != nil {
			return err
		}

		if networkId == "" {
			return fmt.Errorf(errorUnknownNetworkName, d.NetworkName)
		}

		d.NetworkId = networkId
		log.WithFields(log.Fields{
			"Name": d.NetworkName,
			"ID":   d.NetworkId,
		}).Debug("Found network id using its name")
	}

	if d.FlavorName != "" {
		if err := d.initCompute(); err != nil {
			return err
		}
		flavorId, err := d.client.GetFlavorId(d)

		if err != nil {
			return err
		}

		if flavorId == "" {
			return fmt.Errorf(errorUnknownFlavorName, d.FlavorName)
		}

		d.FlavorId = flavorId
		log.WithFields(log.Fields{
			"Name": d.FlavorName,
			"ID":   d.FlavorId,
		}).Debug("Found flavor id using its name")
	}

	if d.ImageName != "" {
		if err := d.initCompute(); err != nil {
			return err
		}
		imageId, err := d.client.GetImageId(d)

		if err != nil {
			return err
		}

		if imageId == "" {
			return fmt.Errorf(errorUnknownImageName, d.ImageName)
		}

		d.ImageId = imageId
		log.WithFields(log.Fields{
			"Name": d.ImageName,
			"ID":   d.ImageId,
		}).Debug("Found image id using its name")
	}

	if d.FloatingIpPool != "" {
		if err := d.initNetwork(); err != nil {
			return err
		}
		f, err := d.client.GetFloatingIpPoolId(d)

		if err != nil {
			return err
		}

		if f == "" {
			return fmt.Errorf(errorUnknownNetworkName, d.FloatingIpPool)
		}

		d.FloatingIpPoolId = f
		log.WithFields(log.Fields{
			"Name": d.FloatingIpPool,
			"ID":   d.FloatingIpPoolId,
		}).Debug("Found floating IP pool id using its name")
	}

	return nil
}

func (d *Driver) initCompute() error {
	if err := d.client.Authenticate(d); err != nil {
		return err
	}
	if err := d.client.InitComputeClient(d); err != nil {
		return err
	}
	return nil
}

func (d *Driver) initNetwork() error {
	if err := d.client.Authenticate(d); err != nil {
		return err
	}
	if err := d.client.InitNetworkClient(d); err != nil {
		return err
	}
	return nil
}

func (d *Driver) createSSHKey() error {
	log.WithField("Name", d.KeyPairName).Debug("Creating Key Pair...")
	if err := ssh.GenerateSSHKey(d.sshKeyPath()); err != nil {
		return err
	}
	publicKey, err := ioutil.ReadFile(d.publicSSHKeyPath())
	if err != nil {
		return err
	}

	if err := d.initCompute(); err != nil {
		return err
	}
	if err := d.client.CreateKeyPair(d, d.KeyPairName, string(publicKey)); err != nil {
		return err
	}
	return nil
}

func (d *Driver) createMachine() error {
	log.WithFields(log.Fields{
		"FlavorId": d.FlavorId,
		"ImageId":  d.ImageId,
	}).Debug("Creating OpenStack instance...")

	if err := d.initCompute(); err != nil {
		return err
	}
	instanceId, err := d.client.CreateInstance(d)
	if err != nil {
		return err
	}
	d.MachineId = instanceId
	return nil
}

func (d *Driver) assignFloatingIp() error {

	if err := d.initNetwork(); err != nil {
		return err
	}

	portId, err := d.client.GetInstancePortId(d)
	if err != nil {
		return err
	}

	ips, err := d.client.GetFloatingIPs(d)
	if err != nil {
		return err
	}

	var floatingIp *FloatingIp

	log.WithFields(log.Fields{
		"MachineId": d.MachineId,
		"Pool":      d.FloatingIpPool,
	}).Debugf("Looking for an available floating IP")

	for _, ip := range ips {
		if ip.PortId == "" {
			log.WithFields(log.Fields{
				"MachineId": d.MachineId,
				"IP":        ip.Ip,
			}).Debugf("Available floating IP found")
			floatingIp = &ip
			break
		}
	}

	if floatingIp == nil {
		floatingIp = &FloatingIp{}
		log.WithField("MachineId", d.MachineId).Debugf("No available floating IP found. Allocating a new one...")
	} else {
		log.WithField("MachineId", d.MachineId).Debugf("Assigning floating IP to the instance")
	}

	if err := d.client.AssignFloatingIP(d, floatingIp, portId); err != nil {
		return err
	}
	d.Ip = floatingIp.Ip
	return nil
}

func (d *Driver) waitForInstanceActive() error {
	log.WithField("MachineId", d.MachineId).Debug("Waiting for the OpenStack instance to be ACTIVE...")
	if err := d.client.WaitForInstanceStatus(d, "ACTIVE", 200); err != nil {
		return err
	}
	return nil
}

func (d *Driver) lookForIpAddress() error {
	ip, err := d.GetIP()
	if err != nil {
		return err
	}
	d.Ip = ip
	log.WithFields(log.Fields{
		"IP":        ip,
		"MachineId": d.MachineId,
	}).Debug("IP address found")
	return nil
}

func (d *Driver) waitForSSHServer() error {
	ip, err := d.GetIP()
	if err != nil {
		return err
	}
	log.WithFields(log.Fields{
		"MachineId": d.MachineId,
		"IP":        ip,
	}).Debug("Waiting for the SSH server to be started...")
	return ssh.WaitForTCP(fmt.Sprintf("%s:%d", ip, d.SSHPort))
}

func (d *Driver) waitForInstanceToStart() error {
	if err := d.waitForInstanceActive(); err != nil {
		return err
	}
	return d.waitForSSHServer()
}

func (d *Driver) installDocker() error {
	log.WithField("MachineId", d.MachineId).Debug("Adding key to authorized-keys.d...")

	if err := drivers.AddPublicKeyToAuthorizedHosts(d, "/.docker/authorized-keys.d"); err != nil {
		return err
	}

	log.WithField("MachineId", d.MachineId).Debug("Installing docker daemon on the machine")

	cmd := "curl -sSL https://gist.githubusercontent.com/smashwilson/1a286139720a28ac6ead/raw/41d93c57ea2e86815cdfbfec42aaa696034afcc8/setup-docker.sh | /bin/bash"
	sshCmd, err := d.GetSSHCommand(cmd)
	if err != nil {
		return err
	}
	if err := sshCmd.Run(); err != nil {
		log.Warnf("Docker installation failed: %v", err)
		log.Warnf("The machine is not ready to run docker containers")
	}
	return nil
}

func (d *Driver) sshKeyPath() string {
	return path.Join(d.storePath, "id_rsa")
}

func (d *Driver) publicSSHKeyPath() string {
	return d.sshKeyPath() + ".pub"
}

func (d *Driver) setMachineNameIfNotSet() {
	if d.MachineName == "" {
		d.MachineName = fmt.Sprintf("docker-host-%s", utils.GenerateRandomID())
	}
}
