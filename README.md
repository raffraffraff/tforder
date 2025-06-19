# OpenTofu stack dependency graph generation
In my [terrahiera](https://github.com/raffraffraff/terrahiera) project I demonstrate an opinionated way to structure Terraform/Tofu repositories. Each of my stack deployment directories contain dependencies declarations like this:

```
locals {
  dependencies = {
    vpc = "../vpc"
    dns = "../../global/dns"
  }
}
```

`tforder` can parse those dependency declarations across your repository and generate a Directed Acyclical Graph of your dependencies. It can output .dot, .svg or .png files. It can also recursively execute commands in each stack, in order of dependency or in _reverse_ order of dependency (for destroying infrastructure)
 
# Usage:
```
tforder \
  -dir <start_dir> \
  [-execute] \
  [-out <file.dot|file.svg|file.png>] \
  [-maxparallel <number>] \
  [-recursive] \
  [-relative-to <base>] \
  [-reverse]
```

## Flags:
*  `-dir`  Directory to start in (default is the current working directory, `.`)
*  `-execute`  Executes a command in each stack directory, in order of dependeies
*  `-out`  Output file (.dot, .svg, .png; default: tforder.dot)
*  `-maxparallel`  When executing commands, set the maximum number of parallel operations
*  `-recursive`  Recursively scan all subdirectories for main.tf files
*  `-relative-to`  Base path for relative node names (default: current working directory)
*  `-reverse`  Reverses the order of the dependency graph (useful for executing TF destroy operations)

## Examples:
### Dependency graph for a specific stack
`tforder -dir dev/eu-west-1/ew1a/eks -out eks.svg`
![graph.svg](https://github.com/raffraffraff/tforder/blob/main/example/graph.svg?raw=true)

### Dependency graph of your whole infrastructure
`tforder -dir dev -recursive -out infra.dot`
```
digraph tforder {
  "dev/eu-west-1/ew1a/vpc" -> "dev/eu-west-1/ew1a/eks";
  "dev/global/shared/apex_zones" -> "dev/eu-west-1/ew1a/vpc";
  "dev/eu-west-1/ew1b/vpc" -> "dev/eu-west-1/ew1b/eks";
  "dev/global/shared/apex_zones" -> "dev/eu-west-1/ew1b/vpc";
}
```

### Dependency graph of your whole infrastructure, with relative dir names
`tforder -dir dev -recursive -out infra.svg -relative-to dev`
![infra.svg](https://github.com/raffraffraff/tforder/blob/main/example/infra.svg?raw=true)

### Deploy all stacks in eu-west-2 in dependency order with up to 3 threads
`tforder -recursive -dir deployments/dev/eu-west-2 -execute 'tofu init && tofu apply -auto-approve' --maxparallel 3`
if terraform show -json | jq '.. | objects | select(has("mode")) | select(.mode=="managed")' | grep -q .; then
  echo "There are managed resources, safe to destroy"
  terraform destroy -auto-approve
else
  echo "No managed resources found, skipping destroy"
fi
