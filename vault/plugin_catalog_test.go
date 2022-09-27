package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/hashicorp/go-hclog"
	semver "github.com/hashicorp/go-version"
	"github.com/hashicorp/vault/api"
	"github.com/hashicorp/vault/builtin/credential/userpass"
	"github.com/hashicorp/vault/helper/versions"
	"github.com/hashicorp/vault/plugins/database/postgresql"
	v5 "github.com/hashicorp/vault/sdk/database/dbplugin/v5"
	"github.com/hashicorp/vault/sdk/helper/consts"
	"github.com/hashicorp/vault/sdk/helper/pluginutil"
	"github.com/hashicorp/vault/sdk/logical"
	backendplugin "github.com/hashicorp/vault/sdk/plugin"

	"github.com/hashicorp/vault/helper/builtinplugins"
)

func TestPluginCatalog_CRUD(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	const pluginName = "mysql-database-plugin"

	// Get builtin plugin
	p, err := core.pluginCatalog.Get(context.Background(), pluginName, consts.PluginTypeDatabase, "")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	expectedBuiltin := &pluginutil.PluginRunner{
		Name:    pluginName,
		Type:    consts.PluginTypeDatabase,
		Builtin: true,
		Version: versions.GetBuiltinVersion(consts.PluginTypeDatabase, pluginName),
	}
	expectedBuiltin.BuiltinFactory, _ = builtinplugins.Registry.Get(pluginName, consts.PluginTypeDatabase)

	if &(p.BuiltinFactory) == &(expectedBuiltin.BuiltinFactory) {
		t.Fatal("expected BuiltinFactory did not match actual")
	}
	expectedBuiltin.BuiltinFactory = nil
	p.BuiltinFactory = nil
	if !reflect.DeepEqual(p, expectedBuiltin) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", p, expectedBuiltin)
	}

	// Set a plugin, test overwriting a builtin plugin
	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	command := filepath.Base(file.Name())
	err = core.pluginCatalog.Set(context.Background(), pluginName, consts.PluginTypeDatabase, "", command, []string{"--test"}, []string{"FOO=BAR"}, []byte{'1'})
	if err != nil {
		t.Fatal(err)
	}

	// Get the plugin
	p, err = core.pluginCatalog.Get(context.Background(), pluginName, consts.PluginTypeDatabase, "")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	expected := &pluginutil.PluginRunner{
		Name:    pluginName,
		Type:    consts.PluginTypeDatabase,
		Command: filepath.Join(tempDir, filepath.Base(file.Name())),
		Args:    []string{"--test"},
		Env:     []string{"FOO=BAR"},
		Sha256:  []byte{'1'},
		Builtin: false,
		Version: "",
	}

	if !reflect.DeepEqual(p, expected) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", p, expected)
	}

	// Delete the plugin
	err = core.pluginCatalog.Delete(context.Background(), pluginName, consts.PluginTypeDatabase, "")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Get builtin plugin
	p, err = core.pluginCatalog.Get(context.Background(), pluginName, consts.PluginTypeDatabase, "")
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	expectedBuiltin = &pluginutil.PluginRunner{
		Name:    pluginName,
		Type:    consts.PluginTypeDatabase,
		Builtin: true,
		Version: versions.GetBuiltinVersion(consts.PluginTypeDatabase, pluginName),
	}
	expectedBuiltin.BuiltinFactory, _ = builtinplugins.Registry.Get(pluginName, consts.PluginTypeDatabase)

	if &(p.BuiltinFactory) == &(expectedBuiltin.BuiltinFactory) {
		t.Fatal("expected BuiltinFactory did not match actual")
	}
	expectedBuiltin.BuiltinFactory = nil
	p.BuiltinFactory = nil
	if !reflect.DeepEqual(p, expectedBuiltin) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", p, expectedBuiltin)
	}
}

func TestPluginCatalog_VersionedCRUD(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	// Set a versioned plugin.
	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	const name = "mysql-database-plugin"
	const version = "1.0.0"
	command := fmt.Sprintf("%s", filepath.Base(file.Name()))
	err = core.pluginCatalog.Set(context.Background(), name, consts.PluginTypeDatabase, version, command, []string{"--test"}, []string{"FOO=BAR"}, []byte{'1'})
	if err != nil {
		t.Fatal(err)
	}

	// Get the plugin
	plugin, err := core.pluginCatalog.Get(context.Background(), name, consts.PluginTypeDatabase, version)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	expected := &pluginutil.PluginRunner{
		Name:    name,
		Type:    consts.PluginTypeDatabase,
		Version: version,
		Command: filepath.Join(tempDir, filepath.Base(file.Name())),
		Args:    []string{"--test"},
		Env:     []string{"FOO=BAR"},
		Sha256:  []byte{'1'},
		Builtin: false,
	}

	if !reflect.DeepEqual(plugin, expected) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", plugin, expected)
	}

	// Also get the builtin version to check we can still access that.
	builtinVersion := versions.GetBuiltinVersion(consts.PluginTypeDatabase, name)
	plugin, err = core.pluginCatalog.Get(context.Background(), name, consts.PluginTypeDatabase, builtinVersion)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}

	expected = &pluginutil.PluginRunner{
		Name:    name,
		Type:    consts.PluginTypeDatabase,
		Version: builtinVersion,
		Builtin: true,
	}

	// Check by marshalling to JSON to avoid messing with BuiltinFactory function field.
	expectedBytes, err := json.Marshal(expected)
	if err != nil {
		t.Fatal(err)
	}
	actualBytes, err := json.Marshal(plugin)
	if err != nil {
		t.Fatal(err)
	}
	if string(expectedBytes) != string(actualBytes) {
		t.Fatalf("expected %s, got %s", string(expectedBytes), string(actualBytes))
	}
	if !plugin.Builtin {
		t.Fatal("expected builtin true but got false")
	}

	// Delete the plugin
	err = core.pluginCatalog.Delete(context.Background(), name, consts.PluginTypeDatabase, version)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	// Get plugin - should fail
	plugin, err = core.pluginCatalog.Get(context.Background(), name, consts.PluginTypeDatabase, version)
	if err != nil {
		t.Fatal(err)
	}
	if plugin != nil {
		t.Fatalf("expected no plugin with this version to be in the catalog, but found %+v", plugin)
	}
}

func TestPluginCatalog_List(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	// Get builtin plugins and sort them
	builtinKeys := builtinplugins.Registry.Keys(consts.PluginTypeDatabase)
	sort.Strings(builtinKeys)

	// List only builtin plugins
	plugins, err := core.pluginCatalog.List(context.Background(), consts.PluginTypeDatabase)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	sort.Strings(plugins)

	if len(plugins) != len(builtinKeys) {
		t.Fatalf("unexpected length of plugin list, expected %d, got %d", len(builtinKeys), len(plugins))
	}

	if !reflect.DeepEqual(plugins, builtinKeys) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", plugins, builtinKeys)
	}

	// Set a plugin, test overwriting a builtin plugin
	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	command := filepath.Base(file.Name())
	err = core.pluginCatalog.Set(context.Background(), "mysql-database-plugin", consts.PluginTypeDatabase, "", command, []string{"--test"}, []string{}, []byte{'1'})
	if err != nil {
		t.Fatal(err)
	}

	// Set another plugin
	err = core.pluginCatalog.Set(context.Background(), "aaaaaaa", consts.PluginTypeDatabase, "", command, []string{"--test"}, []string{}, []byte{'1'})
	if err != nil {
		t.Fatal(err)
	}

	// List the plugins
	plugins, err = core.pluginCatalog.List(context.Background(), consts.PluginTypeDatabase)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	sort.Strings(plugins)

	// plugins has a test-added plugin called "aaaaaaa" that is not built in
	if len(plugins) != len(builtinKeys)+1 {
		t.Fatalf("unexpected length of plugin list, expected %d, got %d", len(builtinKeys)+1, len(plugins))
	}

	// verify the first plugin is the one we just created.
	if !reflect.DeepEqual(plugins[0], "aaaaaaa") {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", plugins[0], "aaaaaaa")
	}

	// verify the builtin plugins are correct
	if !reflect.DeepEqual(plugins[1:], builtinKeys) {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", plugins[1:], builtinKeys)
	}
}

func TestPluginCatalog_ListVersionedPlugins(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	// Get builtin plugins and sort them
	builtinKeys := builtinplugins.Registry.Keys(consts.PluginTypeDatabase)
	sort.Strings(builtinKeys)

	// List only builtin plugins
	plugins, err := core.pluginCatalog.ListVersionedPlugins(context.Background(), consts.PluginTypeDatabase)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	sortVersionedPlugins(plugins)

	if len(plugins) != len(builtinKeys) {
		t.Fatalf("unexpected length of plugin list, expected %d, got %d", len(builtinKeys), len(plugins))
	}

	for i, plugin := range plugins {
		if plugin.Name != builtinKeys[i] {
			t.Fatalf("expected plugin list with names %v but got %+v", builtinKeys, plugins)
		}
	}

	// Set a plugin, test overwriting a builtin plugin
	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	command := filepath.Base(file.Name())
	err = core.pluginCatalog.Set(
		context.Background(),
		"mysql-database-plugin",
		consts.PluginTypeDatabase,
		"",
		command,
		[]string{"--test"},
		[]string{},
		[]byte{'1'},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Set another plugin, with version information
	err = core.pluginCatalog.Set(
		context.Background(),
		"aaaaaaa",
		consts.PluginTypeDatabase,
		"1.1.0",
		command,
		[]string{"--test"},
		[]string{},
		[]byte{'1'},
	)
	if err != nil {
		t.Fatal(err)
	}

	// List the plugins
	plugins, err = core.pluginCatalog.ListVersionedPlugins(context.Background(), consts.PluginTypeDatabase)
	if err != nil {
		t.Fatalf("unexpected error %v", err)
	}
	sortVersionedPlugins(plugins)

	// plugins has a test-added plugin called "aaaaaaa" that is not built in
	if len(plugins) != len(builtinKeys)+1 {
		t.Fatalf("unexpected length of plugin list, expected %d, got %d", len(builtinKeys)+1, len(plugins))
	}

	// verify the first plugin is the one we just created.
	if !reflect.DeepEqual(plugins[0].Name, "aaaaaaa") {
		t.Fatalf("expected did not match actual, got %#v\n expected %#v\n", plugins[0], "aaaaaaa")
	}
	if plugins[0].SemanticVersion == nil {
		t.Fatalf("expected non-nil semantic version for %v", plugins[0].Name)
	}

	// verify the builtin plugins are correct
	for i, plugin := range plugins[1:] {
		if plugin.Name != builtinKeys[i] {
			t.Fatalf("expected plugin list with names %v but got %+v", builtinKeys, plugins)
		}
		switch plugin.Name {
		case "mysql-database-plugin":
			if plugin.Builtin {
				t.Fatalf("expected %v plugin to be an unversioned external plugin", plugin)
			}
			if plugin.Version != "" {
				t.Fatalf("expected no version information for %v but got %s", plugin, plugin.Version)
			}
		default:
			if !plugin.Builtin {
				t.Fatalf("expected %v plugin to be builtin", plugin)
			}
			if plugin.SemanticVersion.Metadata() != "builtin" && plugin.SemanticVersion.Metadata() != "builtin.vault" {
				t.Fatalf("expected +builtin metadata but got %s", plugin.Version)
			}
		}

		if plugin.SemanticVersion == nil {
			t.Fatalf("expected non-nil semantic version for %v", plugin)
		}
	}
}

func TestPluginCatalog_ListHandlesPluginNamesWithSlashes(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	command := filepath.Base(file.Name())
	ctx := context.Background()

	pluginsToRegister := []pluginutil.PluginRunner{
		{
			Name: "unversioned-plugin",
		},
		{
			Name: "unversioned-plugin/with-slash",
		},
		{
			Name: "unversioned-plugin/with-two/slashes",
		},
		{
			Name:    "versioned-plugin",
			Version: "v1.0.0",
		},
		{
			Name:    "versioned-plugin/with-slash",
			Version: "v1.0.0",
		},
		{
			Name:    "versioned-plugin/with-two/slashes",
			Version: "v1.0.0",
		},
	}
	for _, entry := range pluginsToRegister {
		err = core.pluginCatalog.Set(ctx, entry.Name, consts.PluginTypeCredential, entry.Version, command, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}

	plugins, err := core.pluginCatalog.ListVersionedPlugins(ctx, consts.PluginTypeCredential)
	if err != nil {
		t.Fatal(err)
	}

	for _, expected := range pluginsToRegister {
		found := false
		for _, plugin := range plugins {
			if expected.Name == plugin.Name && expected.Version == plugin.Version {
				found = true
				break
			}
		}

		if !found {
			t.Errorf("Did not find %#v in %#v", expected, plugins)
		}
	}
}

func TestPluginCatalog_NoCollisionForUnversionedPlugins(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir
	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	command := filepath.Base(file.Name())

	ctx := context.Background()
	storeEntry := func(entry *pluginutil.PluginRunner) {
		t.Helper()
		err := core.pluginCatalog.Set(ctx, entry.Name, entry.Type, entry.Version, command, nil, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	findEntry := func(entry *pluginutil.PluginRunner) bool {
		t.Helper()
		plugin, err := core.pluginCatalog.Get(ctx, entry.Name, entry.Type, entry.Version)
		if err != nil {
			t.Fatal(err)
		}
		return plugin != nil && plugin.Name == entry.Name && plugin.Type == entry.Type && plugin.Version == entry.Version
	}

	unversionedEntry := &pluginutil.PluginRunner{
		Name:    "my-plugin/v1.0.0",
		Type:    consts.PluginTypeCredential,
		Version: "",
		Command: command,
	}

	storeEntry(unversionedEntry)
	if !findEntry(unversionedEntry) {
		t.Errorf("Did not find %#v in plugins", unversionedEntry)
	}

	versionedEntry := &pluginutil.PluginRunner{
		Name:    "my-plugin",
		Type:    consts.PluginTypeCredential,
		Version: "v1.0.0",
		Command: command,
	}

	storeEntry(versionedEntry)
	if !findEntry(versionedEntry) {
		t.Fatal("Should exist")
	}
	if !findEntry(unversionedEntry) {
		t.Fatal("Should still exist")
	}
}

func TestPluginCatalog_UpgradePlugins(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	file, err := ioutil.TempFile(tempDir, "temp")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	command := filepath.Base(file.Name())

	ctx := context.Background()
	entries := map[string]*pluginutil.PluginRunner{
		// v0 schema entries wouldn't have had type stored in their entry,
		// but we include it here for convenience of testing, so we don't have to
		// answer a Type() gRPC method.
		"v0schema":                                     {Name: "v0schema", Type: consts.PluginTypeCredential},
		"v0schema/with/slashes":                        {Name: "v0schema/with/slashes", Type: consts.PluginTypeCredential},
		"auth/v1schema":                                {Name: "v1schema", Type: consts.PluginTypeCredential},
		"database/v1schema/with/slashes":               {Name: "v1schema/with/slashes", Type: consts.PluginTypeDatabase},
		"v2/secret/my-plugin/unversioned":              {Name: "my-plugin", Type: consts.PluginTypeSecrets, Version: ""},
		"v2/secret/my-plugin/v1.0.0":                   {Name: "my-plugin", Type: consts.PluginTypeSecrets, Version: "v1.0.0"},
		"v2/secret/my-plugin/with/slashes/unversioned": {Name: "my-plugin/with/slashes", Type: consts.PluginTypeSecrets, Version: ""},
		"v2/secret/my-plugin/with/slashes/v2.0.0":      {Name: "my-plugin/with/slashes", Type: consts.PluginTypeSecrets, Version: "v2.0.0"},
	}
	for storageKey, entry := range entries {
		entry.Command = command

		// Store directly using catalogView to emulate storage key schema from
		// an older version of Vault that's been upgraded.
		buf, err := json.Marshal(entry)
		if err != nil {
			t.Fatal(err)
		}

		logicalEntry := logical.StorageEntry{
			Key:   storageKey,
			Value: buf,
		}
		if err := core.pluginCatalog.catalogView.Put(ctx, &logicalEntry); err != nil {
			t.Fatal(err)
		}
	}
	findEntry := func(entry *pluginutil.PluginRunner) bool {
		t.Helper()
		plugin, err := core.pluginCatalog.Get(ctx, entry.Name, entry.Type, entry.Version)
		if err != nil {
			t.Fatal(err)
		}
		return plugin != nil && plugin.Name == entry.Name && plugin.Type == entry.Type && plugin.Version == entry.Version
	}

	// Run a few upgrades to ensure they are idempotent.
	for i := 0; i < 3; i++ {
		if err := core.pluginCatalog.UpgradePlugins(ctx, hclog.Default()); err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if !findEntry(entry) {
				t.Errorf("Did not find %#v", entry)
			}
		}

		keys, err := logical.CollectKeys(ctx, core.pluginCatalog.catalogView)
		if err != nil {
			t.Fatal(err)
		}

		if len(keys) != len(entries) {
			t.Fatalf("Expected %d entries but got %d entries", len(entries), len(keys))
		}

		// Also check the storage is all in the form we expect.
		for _, key := range keys {
			parts := strings.Split(key, "/")
			if len(parts) < 4 {
				t.Fatalf("Key with less than 3 slashes: %s", key)
			}
			if parts[0] != pluginCatalogSchemaVersion {
				t.Errorf("Expected schema version %q at start of key but got %q", pluginCatalogSchemaVersion, parts[0])
			}
			if _, err := consts.ParsePluginType(parts[1]); err != nil {
				t.Errorf("Expected valid plugin type in second segment of key but got %q: %s", parts[1], err)
			}
			version := parts[len(parts)-1]
			if _, err := semver.NewVersion(version); err != nil && version != pluginCatalogUnversionedKey {
				t.Errorf("Expected unversioned key or valid semantic version in last segment of key but got %q: %s", version, err)
			}
		}
	}
}

func TestPluginCatalog_NewPluginClient(t *testing.T) {
	core, _, _ := TestCoreUnsealed(t)
	tempDir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	core.pluginCatalog.directory = tempDir

	if extPlugins := len(core.pluginCatalog.externalPlugins); extPlugins != 0 {
		t.Fatalf("expected externalPlugins map to be of len 0 but got %d", extPlugins)
	}

	// register plugins
	TestAddTestPlugin(t, core, "mux-postgres", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_PostgresMultiplexed", []string{}, "")
	TestAddTestPlugin(t, core, "single-postgres-1", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_Postgres", []string{}, "")
	TestAddTestPlugin(t, core, "single-postgres-2", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_Postgres", []string{}, "")

	TestAddTestPlugin(t, core, "mux-userpass", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_UserpassMultiplexed", []string{}, "")
	TestAddTestPlugin(t, core, "single-userpass-1", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_Userpass", []string{}, "")
	TestAddTestPlugin(t, core, "single-userpass-2", consts.PluginTypeUnknown, "", "TestPluginCatalog_PluginMain_Userpass", []string{}, "")

	var pluginClients []*pluginClient
	// run plugins
	// run "mux-postgres" twice which will start a single plugin for 2
	// distinct connections
	c := TestRunTestPlugin(t, core, consts.PluginTypeDatabase, "mux-postgres")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeDatabase, "mux-postgres")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeDatabase, "single-postgres-1")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeDatabase, "single-postgres-2")
	pluginClients = append(pluginClients, c)

	// run "mux-userpass" twice which will start a single plugin for 2
	// distinct connections
	c = TestRunTestPlugin(t, core, consts.PluginTypeCredential, "mux-userpass")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeCredential, "mux-userpass")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeCredential, "single-userpass-1")
	pluginClients = append(pluginClients, c)
	c = TestRunTestPlugin(t, core, consts.PluginTypeCredential, "single-userpass-2")
	pluginClients = append(pluginClients, c)

	externalPlugins := core.pluginCatalog.externalPlugins
	if len(externalPlugins) != 6 {
		t.Fatalf("expected externalPlugins map to be of len 6 but got %d", len(externalPlugins))
	}

	// check connections map
	expectConnectionLen(t, 2, externalPlugins["mux-postgres"].connections)
	expectConnectionLen(t, 1, externalPlugins["single-postgres-1"].connections)
	expectConnectionLen(t, 1, externalPlugins["single-postgres-2"].connections)
	expectConnectionLen(t, 2, externalPlugins["mux-userpass"].connections)
	expectConnectionLen(t, 1, externalPlugins["single-userpass-1"].connections)
	expectConnectionLen(t, 1, externalPlugins["single-userpass-2"].connections)

	// check multiplexing support
	expectMultiplexingSupport(t, true, externalPlugins["mux-postgres"].multiplexingSupport)
	expectMultiplexingSupport(t, false, externalPlugins["single-postgres-1"].multiplexingSupport)
	expectMultiplexingSupport(t, false, externalPlugins["single-postgres-2"].multiplexingSupport)
	expectMultiplexingSupport(t, true, externalPlugins["mux-userpass"].multiplexingSupport)
	expectMultiplexingSupport(t, false, externalPlugins["single-userpass-1"].multiplexingSupport)
	expectMultiplexingSupport(t, false, externalPlugins["single-userpass-2"].multiplexingSupport)

	// cleanup all of the external plugin processes
	for _, client := range pluginClients {
		client.Close()
	}

	// check that externalPlugins map is cleaned up
	if len(externalPlugins) != 0 {
		t.Fatalf("expected external plugin map to be of len 0 but got %d", len(externalPlugins))
	}
}

func TestPluginCatalog_PluginMain_Userpass(t *testing.T) {
	if os.Getenv(pluginutil.PluginVaultVersionEnv) == "" {
		return
	}

	apiClientMeta := &api.PluginAPIClientMeta{}
	flags := apiClientMeta.FlagSet()
	flags.Parse(os.Args[1:])

	tlsConfig := apiClientMeta.GetTLSConfig()
	tlsProviderFunc := api.VaultPluginTLSProvider(tlsConfig)

	err := backendplugin.Serve(
		&backendplugin.ServeOpts{
			BackendFactoryFunc: userpass.Factory,
			TLSProviderFunc:    tlsProviderFunc,
		},
	)
	if err != nil {
		t.Fatalf("Failed to initialize userpass: %s", err)
	}
}

func TestPluginCatalog_PluginMain_UserpassMultiplexed(t *testing.T) {
	if os.Getenv(pluginutil.PluginVaultVersionEnv) == "" {
		return
	}

	apiClientMeta := &api.PluginAPIClientMeta{}
	flags := apiClientMeta.FlagSet()
	flags.Parse(os.Args[1:])

	tlsConfig := apiClientMeta.GetTLSConfig()
	tlsProviderFunc := api.VaultPluginTLSProvider(tlsConfig)

	err := backendplugin.ServeMultiplex(
		&backendplugin.ServeOpts{
			BackendFactoryFunc: userpass.Factory,
			TLSProviderFunc:    tlsProviderFunc,
		},
	)
	if err != nil {
		t.Fatalf("Failed to initialize userpass: %s", err)
	}
}

func TestPluginCatalog_PluginMain_Postgres(t *testing.T) {
	if os.Getenv(pluginutil.PluginVaultVersionEnv) == "" {
		return
	}

	dbType, err := postgresql.New()
	if err != nil {
		t.Fatalf("Failed to initialize postgres: %s", err)
	}

	v5.Serve(dbType.(v5.Database))
}

func TestPluginCatalog_PluginMain_PostgresMultiplexed(_ *testing.T) {
	if os.Getenv(pluginutil.PluginVaultVersionEnv) == "" {
		return
	}

	v5.ServeMultiplex(postgresql.New)
}

// expectConnectionLen asserts that the PluginCatalog's externalPlugin
// connections map has a length of expectedLen
func expectConnectionLen(t *testing.T, expectedLen int, connections map[string]*pluginClient) {
	if len(connections) != expectedLen {
		t.Fatalf("expected external plugin's connections map to be of len %d but got %d", expectedLen, len(connections))
	}
}

func expectMultiplexingSupport(t *testing.T, expected, actual bool) {
	if expected != actual {
		t.Fatalf("expected external plugin multiplexing support to be %t", expected)
	}
}
