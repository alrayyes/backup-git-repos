// Package backup mirrors git repositories out of self-hosted forges.
//
// A backup has to survive the forge going away, so what it keeps is a bare
// mirror clone of every repository: every branch, every tag, and the namespace
// folder structure the forge used, refreshed in place on later runs. Archived
// and active repositories can be selected separately, and either set can be
// written out as a tar.gz alongside the mirror.
package backup
