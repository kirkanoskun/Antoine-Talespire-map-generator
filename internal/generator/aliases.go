package generator

import "sort"

// propAliases maps friendly / descriptive point-of-interest names (the kind a
// language model or a user is likely to produce, e.g. "broken_pillar") onto the
// concrete prop ids that exist in configs/props.json.
//
// This is a pragmatic Phase 1 bridge: the asset catalogue is fixed and small,
// so descriptions rarely match an id exactly. Unknown props fall through and are
// reported as warnings rather than silently dropped. Extend freely as the
// vocabulary grows.
var propAliases = map[string]string{
	"broken_pillar": "big_stone_wall",
	"pillar":        "big_stone_wall",
	"column":        "big_stone_wall",
	"altar":         "stone_big",
	"campfire":      "fire",
	"bonfire":       "fire",
	"skull":         "bull_skull",
	"skulls":        "pile_of_skulls",
	"skeleton":      "down_skeleton",
	"dead_tree":     "dead_tree_big",
	"crystal":       "small_green_crystal",
	"bush":          "floor_bush",
	"rock":          "stone_big",
	"boulder":       "stone_big",
	"cactus":        "cactus_big",
	"tree":          "one_big_tree",
	"pine":          "pine_tree_big",
}

// resolvePropAlias returns the concrete prop id for a POI name. If the name is
// already a known prop id it is returned unchanged; otherwise the alias table is
// consulted; otherwise the original string is returned (and will be reported as
// unknown by the caller when the repository has no such prop).
func resolvePropAlias(name string) string {
	if id, ok := propAliases[name]; ok {
		return id
	}
	return name
}

// POIAliasNames returns the sorted set of friendly point-of-interest names the
// generator understands (the keys of the alias table). The NL layer (package
// nl) feeds these to the model so it prefers vocabulary that resolves cleanly.
func POIAliasNames() []string {
	names := make([]string, 0, len(propAliases))
	for name := range propAliases {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
