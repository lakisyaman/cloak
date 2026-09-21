//go:build integration

package integration

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lakisyaman/cloak/internal/app"
	"github.com/moby/moby/api/types/container"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestMySQLDNSSRVDoesNotReceiveSavedCredentials(t *testing.T) {
	testcontainers.SkipIfProviderIsNotHealthy(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)

	// Both servers accept the same test credentials, but only the first is
	// configured in Cloak. A UUID in the query result identifies the server
	// that actually authenticated the client.
	saved := StartMySQL(t, ctx)
	other := StartMySQL(t, ctx)
	serverUUID := func(server MySQLContainer) string {
		args := append([]string{"mysql"}, server.Args()...)
		args = append(args, "--batch", "--skip-column-names", "-e", "select @@server_uuid")
		output := execInContainer(t, ctx, server.Container, args)
		for _, line := range strings.Split(output, "\n") {
			if id, err := uuid.Parse(strings.TrimSpace(line)); err == nil {
				return id.String()
			}
		}
		t.Fatalf("server did not return a UUID: %q", output)
		return ""
	}
	savedUUID, otherUUID := serverUUID(saved), serverUUID(other)
	if savedUUID == "" || otherUUID == "" || savedUUID == otherUUID {
		t.Fatalf("expected distinct server UUIDs: saved=%q other=%q", savedUUID, otherUUID)
	}
	otherIP, err := other.Container.ContainerIP(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Share the client's network namespace so its resolver can reach this
	// private DNS server on localhost. Only the disposable container's
	// resolv.conf changes; the host resolver is untouched.
	corefile := fmt.Sprintf(`.:53 {
    errors
    template IN SRV {
        match ^_mysql\._tcp\.other\.test\.$
        answer "{{ .Name }} 60 IN SRV 0 0 3306 other.test."
    }
    hosts {
        %s other.test
    }
    log
}
`, otherIP)
	dns, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		Started: true,
		ContainerRequest: testcontainers.ContainerRequest{
			Image: "coredns/coredns:1.12.0",
			Cmd:   []string{"-conf", "/Corefile"},
			HostConfigModifier: func(config *container.HostConfig) {
				config.NetworkMode = container.NetworkMode("container:" + saved.Container.GetContainerID())
			},
			Files: []testcontainers.ContainerFile{{
				Reader: strings.NewReader(corefile), ContainerFilePath: "/Corefile", FileMode: 0o644,
			}},
			WaitingFor: wait.ForLog("CoreDNS-").WithStartupTimeout(30 * time.Second),
		},
	})
	if err != nil {
		t.Fatalf("start DNS fixture: %v", err)
	}
	t.Cleanup(func() { terminateContainer(t, ctx, dns) })
	execInContainer(t, ctx, saved.Container, []string{"sh", "-c", "printf 'nameserver 127.0.0.1\noptions timeout:1 attempts:2\n' > /etc/resolv.conf"})

	args := []string{"--dns-srv-name=_mysql._tcp.other.test", "--batch", "--skip-column-names", "--connect-timeout=5", "-e", "select @@server_uuid, current_user()"}
	// Positive control proves DNS routing and password authentication work.
	control := append([]string{"mysql", "--user=" + saved.Username, "--password=" + saved.Password}, args...)
	output := execInContainer(t, ctx, saved.Container, control)
	want := otherUUID + "\t" + saved.Username + "@%"
	if !strings.Contains(output, want) {
		t.Fatalf("DNS control did not authenticate on the other server: %q", output)
	}
	// A wrong password must fail, so a later successful query demonstrates
	// use of the saved password rather than a passwordless account.
	control[2] = "--password=deliberately-wrong"
	exitCode, output, err := tryExecInContainer(ctx, saved.Container, control)
	if err != nil || exitCode == 0 || !strings.Contains(output, "ERROR 1045") {
		t.Fatalf("wrong-password control must fail authentication: exit=%d output=%q err=%v", exitCode, output, err)
	}
	// This is also the expected result after passthrough is fixed: the same
	// real client, arguments, and inherited environment have no credentials.
	exitCode, output, err = tryExecInContainer(ctx, saved.Container, append([]string{"mysql"}, args...))
	if err != nil || exitCode == 0 || !strings.Contains(output, "ERROR 1045") {
		t.Fatalf("uncredentialed control must fail authentication: exit=%d output=%q err=%v", exitCode, output, err)
	}

	env, shimPath := newCloakIntegrationEnv(t, "mysql")
	runContextCommand(t, env, "mysql", "context", "configure", "saved",
		"--host", "127.0.0.1", "--port", "3306",
		"--username", saved.Username, "--password", saved.Password)
	runContextCommand(t, env, "mysql", "context", "switch", "saved")
	delegate := &containerDelegate{t: t, ctx: ctx, container: saved.Container}
	var stderr bytes.Buffer
	err = app.ExecuteInvocationWithOptions(app.InvocationOptions{
		Paths: env.Paths, Version: "test", Argv0: shimPath, Args: args,
		Stderr: &stderr, Resolver: env.Resolver, Delegate: delegate,
		Store: env.Store, Secrets: env.Secrets, Env: []string{},
	})
	if strings.Contains(delegate.output, want) {
		t.Fatalf("saved credentials authenticated on the DNS-selected server: saved UUID=%s, other UUID=%s, result=%q", savedUUID, otherUUID, strings.TrimSpace(delegate.output))
	}
	if err == nil || !strings.Contains(delegate.output, "ERROR 1045") {
		t.Fatalf("expected native authentication rejection after passthrough: output=%q err=%v", delegate.output, err)
	}
	if strings.Contains(stderr.String(), "activated mysql context") {
		t.Fatalf("explicit DNS target activated a context: %s", stderr.String())
	}
}
