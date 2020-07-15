package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"gitlab.com/NebulousLabs/errors"
	"gitlab.com/scpcorp/ScPrime/node/api/client"
	"gitlab.com/scpcorp/ScPrime/pubaccesskey"
)

var (
	skykeyCmd = &cobra.Command{
		Use:   "skykey",
		Short: "Perform actions related to Skykeys",
		Long:  `Perform actions related to Skykeys, the encryption keys used for Skyfiles.`,
		Run:   skykeycmd,
	}

	skykeyCreateCmd = &cobra.Command{
		Use:   "create [name]",
		Short: "Create a skykey with the given name.",
		Long: `Create a skykey  with the given name. The --type flag can be
		used to specify the skykey type. Its default is private-id.`,
		Run: wrap(skykeycreatecmd),
	}

	skykeyAddCmd = &cobra.Command{
		Use:   "add [skykey base64-encoded skykey]",
		Short: "Add a base64-encoded skykey to the key manager.",
		Long:  `Add a base64-encoded skykey to the key manager.`,
		Run:   wrap(skykeyaddcmd),
	}

	skykeyGetCmd = &cobra.Command{
		Use:   "get",
		Short: "Get the skykey by its name or id",
		Long:  `Get the base64-encoded skykey using either its name with --name or id with --id`,
		Run:   wrap(skykeygetcmd),
	}

	skykeyGetIDCmd = &cobra.Command{
		Use:   "get-id [name]",
		Short: "Get the skykey id by its name",
		Long:  `Get the base64-encoded skykey id by its name`,
		Run:   wrap(skykeygetidcmd),
	}

	skykeyListCmd = &cobra.Command{
		Use:   "ls",
		Short: "List all skykeys",
		Long:  "List all skykeys. Use with --show-priv-keys to show full encoding with private key also.",
		Run:   wrap(skykeylistcmd),
	}
)

// skykeycmd displays the usage info for the command.
func skykeycmd(cmd *cobra.Command, args []string) {
	cmd.UsageFunc()(cmd)
	os.Exit(exitCodeUsage)
}

// skykeycreatecmd is a wrapper for skykeyCreate used to handle skykey creation.
func skykeycreatecmd(name string) {
	skykeyStr, err := skykeyCreate(httpClient, name, skykeyType)
	if err != nil {
		die(errors.AddContext(err, "Failed to create new skykey"))
	}
	fmt.Printf("Created new skykey: %v\n", skykeyStr)
}

// skykeyCreate creates a new Pubaccesskey with the given name and cipher type
func skykeyCreate(c client.Client, name, skykeyTypeString string) (string, error) {
	var st skykey.SkykeyType
	err := st.FromString(skykeyTypeString)
	if err != nil {
		return "", errors.AddContext(err, "Unable to decode skykey type")
	}

	sk, err := c.SkykeyCreateKeyPost(name, st)
	if err != nil {
		return "", errors.AddContext(err, "Could not create skykey")
	}
	return sk.ToString()
}

// skykeyaddcmd is a wrapper for skykeyAdd used to handle the addition of new skykeys.
func skykeyaddcmd(skykeyString string) {
	err := skykeyAdd(httpClient, skykeyString)
	if err != nil && strings.Contains(err.Error(), skykey.ErrSkykeyWithNameAlreadyExists.Error()) {
		die("Pubaccesskey name already used. Try using the --rename-as parameter with a different name.")
	}
	if err != nil {
		die(errors.AddContext(err, "Failed to add skykey"))
	}

	fmt.Printf("Successfully added new skykey: %v\n", skykeyString)
}

// skykeyAdd adds the given skykey to the renter's skykey manager.
func skykeyAdd(c client.Client, skykeyString string) error {
	var sk skykey.Pubaccesskey
	err := sk.FromString(skykeyString)
	if err != nil {
		return errors.AddContext(err, "Could not decode skykey string")
	}

	// Rename the skykey if the --rename-as flag was provided.
	if skykeyRenameAs != "" {
		sk.Name = skykeyRenameAs
	}

	err = c.SkykeyAddKeyPost(sk)
	if err != nil {
		return errors.AddContext(err, "Could not add skykey")
	}

	return nil
}

// skykeygetcmd is a wrapper for skykeyGet that handles skykey get commands.
func skykeygetcmd() {
	skykeyStr, err := skykeyGet(httpClient, skykeyName, skykeyID)
	if err != nil {
		die(err)
	}

	fmt.Printf("Found skykey: %v\n", skykeyStr)
}

// skykeyGet retrieves the skykey using a name or id flag.
func skykeyGet(c client.Client, name, id string) (string, error) {
	if name == "" && id == "" {
		return "", errors.New("Cannot get skykey without using --name or --id flag")
	}
	if name != "" && id != "" {
		return "", errors.New("Use only one flag to get the skykey: --name or --id flag")
	}

	var sk skykey.Pubaccesskey
	var err error
	if name != "" {
		sk, err = c.SkykeyGetByName(name)
	} else {
		var skykeyID skykey.SkykeyID
		err = skykeyID.FromString(id)
		if err != nil {
			return "", errors.AddContext(err, "Could not decode skykey ID")
		}
		sk, err = c.SkykeyGetByID(skykeyID)
	}

	if err != nil {
		return "", errors.AddContext(err, "Failed to retrieve skykey")
	}

	return sk.ToString()
}

// skykeygetidcmd retrieves the skykey id using its name.
func skykeygetidcmd(skykeyName string) {
	sk, err := httpClient.SkykeyGetByName(skykeyName)
	if err != nil {
		die("Failed to retrieve skykey:", err)
	}
	fmt.Printf("Found skykey ID: %v\n", sk.ID().ToString())
}

// skykeylistcmd is a wrapper for skykeyListKeys that prints a list of all
// skykeys.
func skykeylistcmd() {
	skykeysString, err := skykeyListKeys(httpClient, skykeyShowPrivateKeys)
	if err != nil {
		die("Failed to get all skykeys:", err)
	}
	fmt.Print(skykeysString)
}

// skykeyListKeys returns a formatted string containing a list of all skykeys
// being stored by the renter. It includes IDs, Names, and if showPrivateKeys is
// set to true it will include the full encoded skykey.
func skykeyListKeys(c client.Client, showPrivateKeys bool) (string, error) {
	skykeys, err := c.SkykeySkykeysGet()
	if err != nil {
		return "", err
	}

	var b strings.Builder
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)

	// Print a title row.
	if showPrivateKeys {
		fmt.Fprintf(w, "ID\tName\tType\tFull Pubaccesskey\n")
	} else {
		fmt.Fprintf(w, "ID\tName\tType\n")
	}

	if err = w.Flush(); err != nil {
		return "", err
	}
	titleLen := b.Len() - 1
	for i := 0; i < titleLen; i++ {
		fmt.Fprintf(w, "-")
	}
	fmt.Fprintf(w, "\n")

	for _, sk := range skykeys {
		idStr := sk.ID().ToString()
		if !showPrivateKeys {
			fmt.Fprintf(w, "%s\t%s\t%s\n", idStr, sk.Name, sk.Type.ToString())
			continue
		}
		skStr, err := sk.ToString()
		if err != nil {
			return "", err
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", idStr, sk.Name, sk.Type.ToString(), skStr)
	}

	if err = w.Flush(); err != nil {
		return "", err
	}
	return b.String(), nil
}
