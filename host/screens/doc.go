// Package screens holds the shapes the Map, the Reader and Decide render, as
// JSON. The desktop backend and the Bowrain server both produce them, and the
// views in packages/project-screens read nothing else. The package imports
// only core/, so a server that cannot import the host root can build them.
//
// The fixtures under packages/project-screens/fixtures are these types
// marshalled; TestFixtures fails when one of them differs.
package screens
