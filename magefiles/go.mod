module github.com/mesh-intelligence/cobbler-scaffold/magefiles

go 1.25.7

require (
	github.com/magefile/mage v1.15.0
	github.com/mesh-intelligence/cobbler-scaffold v0.20260222.1
)

require gopkg.in/yaml.v3 v3.0.1 // indirect

replace github.com/mesh-intelligence/cobbler-scaffold => ../
