package stack

import (
	"golang.org/x/net/context"

	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/compose/convert"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/swarm"
)

func deployBundle(ctx context.Context, dockerCli command.Cli, opts deployOptions) error {
	bundle, err := loadBundlefile(dockerCli.Err(), opts.namespace, opts.bundlefile)
	if err != nil {
		return err
	}

	if err := checkDaemonIsSwarmManager(ctx, dockerCli); err != nil {
		return err
	}

	namespace := convert.NewNamespace(opts.namespace)

	if opts.prune {
		services := map[string]struct{}{}
		for service := range bundle.Services {
			services[service] = struct{}{}
		}
		pruneServices(ctx, dockerCli, namespace, services)
	}

	networks := make(map[string]types.NetworkCreate)
	for _, service := range bundle.Services {
		for _, networkName := range service.Networks {
			networks[networkName] = types.NetworkCreate{
				Labels: convert.AddStackLabel(namespace, nil),
			}
		}
	}

	services := make(map[string]swarm.ServiceSpec)
	for internalName, service := range bundle.Services {
		name := namespace.Scope(internalName)

		var ports []swarm.PortConfig
		for _, portSpec := range service.Ports {
			ports = append(ports, swarm.PortConfig{
				Protocol:   swarm.PortConfigProtocol(portSpec.Protocol),
				TargetPort: portSpec.Port,
			})
		}

		nets := []swarm.NetworkAttachmentConfig{}
		for _, networkName := range service.Networks {
			nets = append(nets, swarm.NetworkAttachmentConfig{
				Target:  namespace.Scope(networkName),
				Aliases: []string{internalName},
			})
		}

		serviceSpec := swarm.ServiceSpec{
			Annotations: swarm.Annotations{
				Name:   name,
				Labels: convert.AddStackLabel(namespace, service.Labels),
			},
			TaskTemplate: swarm.TaskSpec{
				ContainerSpec: swarm.ContainerSpec{
					Image:   service.Image,
					Command: service.Command,
					Args:    service.Args,
					Env:     service.Env,
					// Service Labels will not be copied to Containers
					// automatically during the deployment so we apply
					// it here.
					Labels: convert.AddStackLabel(namespace, nil),
				},
			},
			EndpointSpec: &swarm.EndpointSpec{
				Ports: ports,
			},
			Networks: nets,
		}

		services[internalName] = serviceSpec
	}

	if err := createNetworks(ctx, dockerCli, namespace, networks); err != nil {
		return err
	}
	return deployServices(ctx, dockerCli, services, namespace, opts.sendRegistryAuth, opts.resolveImage)
}
