package atomfs

import (
	"crypto/sha256"
	"fmt"
	"io"
	"io/ioutil"
	"os"

	"github.com/anuvu/atomfs/db"
	"github.com/anuvu/atomfs/types"
)

type Instance struct {
	config types.Config
	db     *db.AtomfsDB
}

func New(config types.Config) (*Instance, error) {
	if err := os.MkdirAll(config.Path, 0755); err != nil {
		if !os.IsExist(err) {
			return nil, err
		}
	}

	db, err := db.New(config)
	if err != nil {
		return nil, err
	}

	return &Instance{config: config, db: db}, nil
}

func (atomfs *Instance) Close() error {
	return atomfs.db.Close()
}

// FSCK does a filesystem check on this atomfs instance, returning any errors.
func (atomfs *Instance) FSCK() ([]string, error) {
	atoms, err := atomfs.db.GetAtoms()
	if err != nil {
		return nil, err
	}

	errs := []string{}

	// TODO, we could do progress here.
	for _, atom := range atoms {
		f, err := os.Open(atomfs.config.AtomsPath(atom.Hash))
		if err != nil {
			// TODO: should check and see if this atom is used in
			// any molecules, and if so delete those molecules,
			// and if not at least delete it from the db.
			errs = append(errs, err.Error())
			continue
		}

		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}

		// Uh oh. Again, we should try to prune this, perhaps based on
		// some "fix" parameter.
		if fmt.Sprintf("%x", h.Sum(nil)) != atom.Hash {
			errs = append(errs, fmt.Sprintf("%s does not match its hash", atom.Hash))
		}
	}

	return errs, nil
}

// GC does a garbage collection of atomfs, deleting any unused atoms, and any
// files in the atom directory that aren't in the database.
func (atomfs *Instance) GC(dryRun bool) error {
	// First, let's prune unused atoms from the DB.
	unusedAtoms, err := atomfs.db.GetUnusedAtoms()
	if err != nil {
		return err
	}

	if !dryRun {
		for _, atom := range unusedAtoms {
			if err := atomfs.db.DeleteThing(atom.ID, "atom"); err != nil {
				return err
			}
		}
	}

	// Now, delete everything that's on disk that isn't in our DB.
	onDiskAtoms, err := ioutil.ReadDir(atomfs.config.AtomsPath())
	if err != nil {
		return err
	}

	inDBAtoms, err := atomfs.db.GetAtoms()
	if err != nil {
		return err
	}

	for _, onDiskAtom := range onDiskAtoms {
		found := false
		for _, inDBAtom := range inDBAtoms {
			if onDiskAtom.Name() == inDBAtom.Hash {
				found = true
				break
			}
		}

		if !found && !dryRun {
			err := os.Remove(atomfs.config.AtomsPath(onDiskAtom.Name()))
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// CopyMolecule simply duplicates a molecule's configuration under a new name.
// This is equivalent to a "snapshot" operation under other filesystems.
func (atomfs *Instance) CopyMolecule(dest string, src string) (types.Molecule, error) {
	mol, err := atomfs.db.GetMolecule(src)
	if err != nil {
		return types.Molecule{}, err
	}

	return atomfs.db.CreateMolecule(dest, mol.Atoms)
}

func (atomfs *Instance) DeleteMolecule(name string) error {
	mol, err := atomfs.db.GetMolecule(name)
	if err != nil {
		return err
	}

	return atomfs.db.DeleteThing(mol.ID, "molecule")
}
