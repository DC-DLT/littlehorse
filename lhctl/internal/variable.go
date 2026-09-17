/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>
*/
package internal

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/littlehorse-enterprises/littlehorse/sdk-go/lhproto"
	"github.com/littlehorse-enterprises/littlehorse/sdk-go/littlehorse"

	"github.com/spf13/cobra"
)

// getVariableCmd represents the variable command
var getVariableCmd = &cobra.Command{
	Use:   "variable <wfRunId> <threadRunNumber> <varName>",
	Short: "Get a VariableValue by identifiers.",
	Long: `VariableValues's are identified uniquely by the combination of the following:
	- Associated WfRun Id
	- Thread Run Number
	- Variable Name

	You may provide all three identifiers as three separate arguments or you may provide
	them delimited by the '/' character, as returned in all 'search' command queries.
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		needsHelp := false
		if len(args) == 1 {
			args = strings.Split(args[0], "/")
		}

		if len(args) != 1 && len(args) != 3 {
			needsHelp = true
		}

		if needsHelp {
			return errors.New("must provide 1 or 3 arguments. See 'lhctl get variable -h'")
		}

		return nil
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 1 {
			args = strings.Split(args[0], "/")
		}

		threadRunNumber, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatal("Failed parsing threadRunNumber" + err.Error())

		}

		littlehorse.PrintResp(
			getGlobalClient(cmd).GetVariable(
				requestContext(cmd),
				&lhproto.VariableId{
					WfRunId:         littlehorse.StrToWfRunId(args[0]),
					ThreadRunNumber: int32(threadRunNumber),
					Name:            args[2],
				},
			),
		)
	},
}

var searchVariableCmd = &cobra.Command{
	Use:   "variable",
	Short: "Search for Variables by their value",
	Long: `
Search for variables by specifying the value
Search for Variable's by providing the WfRunId OR by specifying the name, type, and
value of variable to search for.

Returns a list of ObjectId's that can be passed into 'lhctl get variable'.

Choose one of the following option groups:
[wfRunId]
[varType, value, name, wfSpecName, wfSpecVersion]
`,
	Args: cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {

		bookmark, _ := cmd.Flags().GetBytesBase64("bookmark")
		limit, _ := cmd.Flags().GetInt32("limit")

		var search lhproto.SearchVariableRequest
		name, _ := cmd.Flags().GetString("name")
		varTypeStr, _ := cmd.Flags().GetString("varType")
		valueStr, _ := cmd.Flags().GetString("value")
		wfSpecName, _ := cmd.Flags().GetString("wfSpecName")

		var wfSpecMajorVersion *int32 = nil
		var wfSpecRevision *int32 = nil

		majorVersionRaw, _ := cmd.Flags().GetInt32("wfSpecMajorVersion")
		if majorVersionRaw != -1 {
			wfSpecMajorVersion = &majorVersionRaw
		}

		revisionRaw, _ := cmd.Flags().GetInt32("wfSpecRevision")
		if revisionRaw != -1 {
			wfSpecRevision = &revisionRaw
		}

		varType, validVarType := lhproto.VariableType_value[varTypeStr]
		if !validVarType {
			log.Fatal(
				"Unrecognized varType. Valid options: INT, STR, BYTES, BOOL, JSON_OBJ, JSON_ARR, DOUBLE.",
			)

		}
		varTypeEnum := lhproto.VariableType(varType)
		content, err := littlehorse.StrToVarVal(valueStr, varTypeEnum)
		if err != nil {
			log.Fatal("Failed deserializing payload: " + err.Error())

		}

		search = lhproto.SearchVariableRequest{
			Value:              content,
			VarName:            name,
			WfSpecMajorVersion: wfSpecMajorVersion,
			WfSpecRevision:     wfSpecRevision,
			WfSpecName:         wfSpecName,
		}

		search.Bookmark = bookmark
		search.Limit = &limit

		littlehorse.PrintResp(
			getGlobalClient(cmd).SearchVariable(requestContext(cmd), &search),
		)
	},
}

var listVariableCmd = &cobra.Command{
	Use:   "variable <wfRunId>",
	Short: "List all Variable's for a given WfRun Id.",
	Long: `
Lists all Variable's for a given WfRun Id.
`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		bookmark, _ := cmd.Flags().GetBytesBase64("bookmark")
		limit, _ := cmd.Flags().GetInt32("limit")

		if len(args) != 1 {
			log.Fatal("Must provide one arg: the WfRun ID!")
		}
		wfRunId := args[0]

		req := &lhproto.ListVariablesRequest{
			WfRunId:  littlehorse.StrToWfRunId(wfRunId),
			Bookmark: bookmark,
			Limit:    &limit,
		}

		littlehorse.PrintResp(getGlobalClient(cmd).ListVariables(
			requestContext(cmd),
			req,
		))
	},
}

var putVariableCmd = &cobra.Command{
	Use:   "variable <wfRunId> <threadRunNumber> <varName> <newValue>",
	Short: "Modify the value of a Variable in a WfRun.",
	Long: `Modifies the value of a Variable belonging to a specific ThreadRun of a WfRun.

Variables are identified uniquely by the combination of the following:
	- Associated WfRun Id
	- Thread Run Number
	- Variable Name

You may provide all three identifiers as three separate arguments followed by the new
value, or you may provide the identifiers delimited by the '/' character (as returned by
all 'search' command queries) followed by the new value.

The new value is intelligently deserialized according to the type declared for that
Variable in the WfSpec; for example, if var 'foo' is of type 'JSON_OBJ', then the
argument '{"bar":"baz"}' is unmarshalled as a JSON object. Pass --varType to skip the
WfSpec lookup and force a specific type.

Once the Variable is updated, the WfRun is advanced, so anything waiting on that Variable
(for example a WAIT_FOR_CONDITION node) reacts to the new value immediately. Subsequent
calls to 'lhctl get variable' return the new value.

For example:

lhctl put variable 2e4e844b3fc9490cbc9d3d0b7d3b6cef 0 foo '{"bar":"baz"}'
lhctl put variable 2e4e844b3fc9490cbc9d3d0b7d3b6cef/0/foo '{"bar":"baz"}'
	`,
	Args: func(cmd *cobra.Command, args []string) error {
		if len(args) == 2 && len(strings.Split(args[0], "/")) == 3 {
			return nil
		}

		if len(args) == 4 {
			return nil
		}

		return errors.New("must provide 2 or 4 arguments. See 'lhctl put variable -h'")
	},
	Run: func(cmd *cobra.Command, args []string) {
		if len(args) == 2 {
			args = append(strings.Split(args[0], "/"), args[1])
		}

		threadRunNumber, err := strconv.Atoi(args[1])
		if err != nil {
			log.Fatal("Failed parsing threadRunNumber: " + err.Error())
		}

		wfRunId := littlehorse.StrToWfRunId(args[0])
		varName := args[2]
		rawValue := args[3]

		client := getGlobalClient(cmd)
		ctx := requestContext(cmd)

		var newValue *lhproto.VariableValue

		if varTypeStr, _ := cmd.Flags().GetString("varType"); varTypeStr != "" {
			varType, validVarType := lhproto.VariableType_value[varTypeStr]
			if !validVarType {
				log.Fatal(
					"Unrecognized varType. Valid options: INT, STR, BYTES, BOOL, JSON_OBJ, JSON_ARR, DOUBLE.",
				)
			}
			newValue, err = littlehorse.StrToVarVal(rawValue, lhproto.VariableType(varType))
		} else {
			newValue, err = resolveNewVariableValue(
				cmd, wfRunId, int32(threadRunNumber), varName, rawValue,
			)
		}

		if err != nil {
			log.Fatal("Failed converting variable value: " + err.Error())
		}

		littlehorse.PrintResp(client.PutVariable(ctx, &lhproto.PutVariableRequest{
			Id: &lhproto.VariableId{
				WfRunId:         wfRunId,
				ThreadRunNumber: int32(threadRunNumber),
				Name:            varName,
			},
			Value: newValue,
		}))
	},
}

// resolveNewVariableValue deserializes rawValue according to the type declared for
// varName in the WfSpec of the provided WfRun. If the WfSpec does not declare the
// Variable, we fall back to STR and warn the user, since 'lhctl put variable' also
// allows setting Variables that the WfSpec does not know about.
func resolveNewVariableValue(
	cmd *cobra.Command,
	wfRunId *lhproto.WfRunId,
	threadRunNumber int32,
	varName string,
	rawValue string,
) (*lhproto.VariableValue, error) {
	client := getGlobalClient(cmd)
	ctx := requestContext(cmd)

	wfRun, err := client.GetWfRun(ctx, wfRunId)
	if err != nil {
		return nil, fmt.Errorf(
			"could not look up WfRun to determine the type of variable '%s' (you may pass --varType instead): %w",
			varName, err,
		)
	}

	var threadRun *lhproto.ThreadRun
	for _, candidate := range wfRun.ThreadRuns {
		if candidate.Number == threadRunNumber {
			threadRun = candidate
			break
		}
	}

	if threadRun == nil {
		return nil, fmt.Errorf("WfRun has no active ThreadRun with number %d", threadRunNumber)
	}

	wfSpecId := threadRun.WfSpecId
	if wfSpecId == nil {
		wfSpecId = wfRun.WfSpecId
	}

	wfSpec, err := client.GetWfSpec(ctx, wfSpecId)
	if err != nil {
		return nil, fmt.Errorf(
			"could not look up WfSpec to determine the type of variable '%s' (you may pass --varType instead): %w",
			varName, err,
		)
	}

	varDef := lookupVarDef(wfRun, wfSpec, threadRun, varName)
	if varDef == nil {
		log.Printf(
			"WARNING: WfSpec '%s' does not declare variable '%s'; sending the value as a STR. "+
				"Pass --varType to choose a different type.",
			wfSpec.Id.Name, varName,
		)
		return littlehorse.StrToVarVal(rawValue, lhproto.VariableType_STR)
	}

	if varDef.TypeDef != nil {
		structDefCache := make(map[string]*lhproto.StructDef)
		structDefResolver := func(id *lhproto.StructDefId) (*lhproto.StructDef, error) {
			cacheKey := id.GetName() + ":" + strconv.Itoa(int(id.GetVersion()))
			if cached, ok := structDefCache[cacheKey]; ok {
				return cached, nil
			}
			structDef, err := client.GetStructDef(ctx, id)
			if err != nil {
				return nil, err
			}
			structDefCache[cacheKey] = structDef
			return structDef, nil
		}
		return littlehorse.TypeDefToVarValWithResolver(rawValue, varDef.TypeDef, structDefResolver)
	}

	if varDef.Type != nil {
		return littlehorse.StrToVarVal(rawValue, *varDef.Type)
	}

	return nil, fmt.Errorf("variable '%s' has no type information in WfSpec", varName)
}

// lookupVarDef finds the VariableDef for varName, starting at the provided ThreadRun and
// walking up its parents. This mirrors how the server resolves which ThreadRun owns a
// Variable.
func lookupVarDef(
	wfRun *lhproto.WfRun,
	wfSpec *lhproto.WfSpec,
	threadRun *lhproto.ThreadRun,
	varName string,
) *lhproto.VariableDef {
	for threadRun != nil {
		if threadSpec := wfSpec.ThreadSpecs[threadRun.ThreadSpecName]; threadSpec != nil {
			for _, threadVarDef := range threadSpec.VariableDefs {
				if threadVarDef.VarDef != nil && threadVarDef.VarDef.Name == varName {
					return threadVarDef.VarDef
				}
			}
		}

		if threadRun.ParentThreadId == nil {
			return nil
		}

		var parent *lhproto.ThreadRun
		for _, candidate := range wfRun.ThreadRuns {
			if candidate.Number == *threadRun.ParentThreadId {
				parent = candidate
				break
			}
		}
		threadRun = parent
	}

	return nil
}

func init() {
	getCmd.AddCommand(getVariableCmd)
	searchCmd.AddCommand(searchVariableCmd)
	listCmd.AddCommand(listVariableCmd)
	putCmd.AddCommand(putVariableCmd)

	searchVariableCmd.Flags().String("varType", "", "type of Variable you're searching for")
	searchVariableCmd.Flags().String("value", "", "value of variable to search for")
	searchVariableCmd.Flags().String("name", "", "name of the variable to search for")
	searchVariableCmd.Flags().String("wfSpecName", "", "name of WfSpec")

	// optional params
	searchVariableCmd.Flags().Int32("wfSpecMajorVersion", -1, "Major Version of WfSpec for Variables to search for")
	searchVariableCmd.Flags().Int32("wfSpecRevision", -1, "Revision of WfSpec for Variables to search for")

	searchVariableCmd.MarkFlagRequired("value")
	searchVariableCmd.MarkFlagRequired("name")
	searchVariableCmd.MarkFlagRequired("varType")
	searchVariableCmd.MarkFlagRequired("wfSpecName")

	putVariableCmd.Flags().String(
		"varType",
		"",
		"Force the type of the new value instead of looking it up in the WfSpec"+
			" (INT, STR, BYTES, BOOL, JSON_OBJ, JSON_ARR, DOUBLE)",
	)
}
