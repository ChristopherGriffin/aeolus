//go:build !unix

package main

// checkOwner has nothing to check off Unix, where the manager is not run.
func checkOwner(string) error { return nil }
