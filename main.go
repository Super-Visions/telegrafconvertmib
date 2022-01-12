package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/template"
	"time"

	"github.com/sleepinggenius2/gosmi"
	"github.com/sleepinggenius2/gosmi/types"
)

type stringArray []string

func (a stringArray) String() string {
	return strings.Join(a, ",")
}

func (a *stringArray) Set(value string) error {
	*a = append(*a, value)
	return nil
}

type trapConfig struct {
	Module string
	Fields stringArray
	Enums  []trapField
	Traps  []trapConfigTrap
}

type trapConfigTrap struct {
	Name        string
	Oid         types.Oid
	Fields      stringArray
	Description string
}

type trapField struct {
	Module string
	Name   string
	Oid    types.Oid
	Values map[int64]string
}

func main() {
	var err error
	var config trapConfig

	// Parse commandline flags
	paths := stringArray{"/usr/share/snmp/mibs"}
	flag.Var(&paths, "p", "Path to add")
	module := flag.String("m", "REQUIRED", "Module to process")
	directory := flag.String("d", "", "Directory to save generated config file. (defaults to stdout if none given)")
	flag.Parse()

	// Load module
	InitSMI(paths)
	config.Module, err = gosmi.LoadModule(*module)
	if err != nil {
		log.Printf("Loading failed: %v", err)
		os.Exit(2)
	}

	// Get traps from module
	trapNodes, err := FindModuleTraps(config.Module)
	if err != nil {
		log.Printf("Finding traps failed: %v", err)
		os.Exit(2)
	}
	if len(trapNodes) == 0 {
		log.Printf("No traps found in module %s", config.Module)
		os.Exit(1)
	}
	config.Traps = make([]trapConfigTrap, 0, len(trapNodes))
	for _, node := range trapNodes {
		trapConfig := ParseTrapToConfig(node)
		config.Traps = append(config.Traps, trapConfig)
		log.Printf("Trap %s::%s [%v]\n", config.Module, trapConfig.Name, trapConfig.Fields)
	}

	// Get list of fields to convert/translate
	fields := GetAllTrapFields(trapNodes)
	config.Fields = make([]string, 0, len(fields))
	for fieldName, trapField := range fields {
		config.Fields = append(config.Fields, fieldName)

		if len(trapField.Values) > 0 {
			config.Enums = append(config.Enums, trapField)
		}
	}

	// Generate telegraf config file
	configFunctions := template.FuncMap{
		"join": strings.Join,
		"now":  time.Now,
	}
	t, err := template.New("telegraf.toml.tmpl").Funcs(configFunctions).ParseFiles("telegraf.toml.tmpl")
	if err != nil {
		log.Printf("Template parsing failed: %v", err)
		os.Exit(2)
	}
	var wr *os.File
	if *directory != "" {
		wr, err = os.Create(*directory + string(os.PathSeparator) + config.Module + ".conf")
		if err != nil {
			log.Printf("Creating config file failed: %v", err)
			os.Exit(2)
		}
	} else {
		wr = os.Stdout
	}
	if err = t.Execute(wr, config); err != nil {
		log.Printf("Generating config failed: %v", err)
		os.Exit(2)
	}

	wr.Close()
}

func InitSMI(paths stringArray) {
	gosmi.Init()

	gosmi.SetPath(strings.Join(paths, string(os.PathListSeparator)))
}

func FindModuleTraps(module string) ([]gosmi.SmiNode, error) {
	m, err := gosmi.GetModule(module)
	if err != nil {
		return nil, fmt.Errorf("cannot get module %s: %w", module, err)
	}

	var trapNodes []gosmi.SmiNode
	for _, node := range m.GetNodes() {
		if node.Kind == types.NodeNotification {
			trapNodes = append(trapNodes, node)
		}
	}

	return trapNodes, nil
}

func ParseTrapToConfig(trap gosmi.SmiNode) (config trapConfigTrap) {
	config.Name = trap.Name
	config.Description = trap.Description
	config.Oid = trap.Oid

	trapFields := trap.GetNotificationObjects()
	config.Fields = make(stringArray, len(trapFields))
	for i, field := range trapFields {
		config.Fields[i] = field.Name
	}

	return config
}

func GetAllTrapFields(nodes []gosmi.SmiNode) map[string]trapField {
	trapFields := make(map[string]trapField)
	for _, trapNode := range nodes {
		for _, objectNode := range trapNode.GetNotificationObjects() {
			trapFields[objectNode.Name] = trapField{
				Name:   objectNode.Name,
				Module: objectNode.GetModule().Name,
				Oid:    objectNode.Oid,
				Values: make(map[int64]string),
			}

			if objectNode.Type.BaseType == types.BaseTypeEnum {
				for _, value := range objectNode.Type.Enum.Values {
					trapFields[objectNode.Name].Values[value.Value] = value.Name
				}
			}
		}
	}
	return trapFields
}
