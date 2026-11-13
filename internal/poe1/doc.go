// Package poe1 holds Path of Exile 1: the engine stats of the character sheet, the rules that
// compute what a modifier cannot express, the output catalogue, the stat semantics table
// authored from game data (table.json), and the predicate that says which stats feed the sheet.
//
// Where neither game text nor game data states a formula detail, such as rounding, the rules
// follow Path of Building as a starting point and mark it unvalidated.
package poe1
