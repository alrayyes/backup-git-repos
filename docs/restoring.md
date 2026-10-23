# Restoring a repository

A `.tar.gz` holds the same bare mirror as the `.git` directory next to it,
not a working-tree checkout -- extracting it alone doesn't hand you files
you can edit. That's deliberate: a bare mirror is the only form that keeps
every branch and tag, not just whichever one would've been checked out.
`git clone` is what turns either one into an ordinary working copy.

A mirror is a normal bare repository, so cloning out of it is the whole
restore:

```bash
git clone /srv/backups/git/home/team/repo.git restored-repo
```

From an archive, extract first:

```bash
tar xzf /srv/backups/git/archive/home/team/repo.tar.gz
git clone repo.git restored-repo
```

If the repository used Git LFS, the restoring machine needs `git-lfs`
installed too -- `git clone` checks out the working tree with plain pointer
files otherwise, and `git lfs pull` afterward is what turns them into the
real file contents, the same way it would against the original forge.
