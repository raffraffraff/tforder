# Terraform dependency graph generation
I follow some crazy patterns in my Terraform. One of them is the specific declaration of each deployment's dependencies, like this:

```
locals {
  dependencies = {
    vpc = "../vpc"
    dns = "../../global/dns"
  }
}
```

My boilerplate Terraform code automatically locates the outputs from those deployments for use in the current deployment. But they are also useful for generating a dependency graph! If you use Terragrunt, you could probably modify the code to work with its dependency declarations (and maybe I'll even do that, but maybe terragrunt does that by itself? I don't know...)

You can point tforder to a specific deployment, or let it recursively find all of your deployments and map out dependencies for your whole infrastructure.

# Usage:
```
tforder -dir <start_dir> [-out <file.dot|file.svg|file.png>] [-relative-to <base>] [-recursive]
```

## Flags:
  -dir           Directory to start in (default: .)
  -out           Output file (.dot, .svg, .png; default: tforder.dot)
  -relative-to   Base path for relative node names (default: current working directory)
  -recursive     Recursively scan all subdirectories for main.tf files

## Examples:
`tforder -dir dev/eu-west-1/ew1a/eks -out eks.svg`
![eks.svg](https://github.com/raffraffraff/tforder/blob/main/example/eks.svg?raw=true)

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
