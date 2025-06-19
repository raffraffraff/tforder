# OpenTofu stack dependency graph generation
I follow some patterns in my TF code. For example, I declare each deployment's dependencies, like this:

```
locals {
  dependencies = {
    vpc = "../vpc"
    dns = "../../global/dns"
  }
}
```

My boilerplate TF code automatically loads outputs from those stacks. As a side effect, these declarations are useful for generating a dependency graph! If you use Terragrunt, you could probably modify the code to work with its dependency declarations. You can point tforder to a specific deployment, or let it recursively find all of your deployments and map out dependencies for your whole infrastructure.

# Usage:
```
tforder -dir <start_dir> [-out <file.dot|file.svg|file.png>] [-relative-to <base>] [-recursive]
```

## Flags:
*  `-dir`  Directory to start in (default is the current working directory, `.`)
*  `-out`  Output file (.dot, .svg, .png; default: tforder.dot)
*  `-relative-to`  Base path for relative node names (default: current working directory)
*  `-recursive`  Recursively scan all subdirectories for main.tf files
*  `-execute`  Executes a command in each stack directory, in order of dependeies
*  `-reverse`  Reverses the order of the dependency graph (useful for executing TF destroy operations)
*  `-maxparallel`  When executing commands, set the maximum number of parallel operations

## Examples:
`tforder -dir dev/eu-west-1/ew1a/eks -out eks.svg`
![graph.svg](https://github.com/raffraffraff/tforder/blob/main/example/graph.svg?raw=true)

`tforder -dir dev -recursive -out infra.dot`
```
digraph tforder {
  "dev/eu-west-1/ew1a/vpc" -> "dev/eu-west-1/ew1a/eks";
  "dev/global/shared/apex_zones" -> "dev/eu-west-1/ew1a/vpc";
  "dev/eu-west-1/ew1b/vpc" -> "dev/eu-west-1/ew1b/eks";
  "dev/global/shared/apex_zones" -> "dev/eu-west-1/ew1b/vpc";
}
```

`tforder -dir dev -recursive -out infra.svg -relative-to dev`
![infra.svg](https://github.com/raffraffraff/tforder/blob/main/example/infra.svg?raw=true)

`tforder -recursive -dir deployments/dev/eu-west-2 -execute 'tofu init && tofu apply -auto-approve' --maxparallel 3`
if terraform show -json | jq '.. | objects | select(has("mode")) | select(.mode=="managed")' | grep -q .; then
  echo "There are managed resources, safe to destroy"
  terraform destroy -auto-approve
else
  echo "No managed resources found, skipping destroy"
fi
