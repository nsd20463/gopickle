// Copyright 2020 NLP Odyssey Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package pytorch

import (
	"archive/tar"
	"archive/zip"
	"errors"
	"fmt"
	"github.com/nlpodyssey/gopickle"
	"github.com/nlpodyssey/gopickle/types"
	"io"
	"math/big"
	"os"
)

const hexMagicNumber = "1950a86a20f9469cfc6c"
const protocolVersion = 1001

var ErrInvalidMagicNumber = errors.New("invalid pytorch magic number")
var ErrInvalidProtocolVersion = errors.New("invalid pytorch protocol version")

func Load(filename string) (interface{}, error) {
	if !isZipFile(filename) {
		return loadLegacyFile(filename)
	}
	return loadZipFile(filename)
}

func loadZipFile(filename string) (interface{}, error) {
	// TODO: ...
	panic("loadZipFile not implemented")
}

func loadLegacyFile(filename string) (interface{}, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		_, err := tr.Next()
		switch err {
		case io.EOF:
			break // End of archive
		case tar.ErrHeader:
			_, err = f.Seek(0, io.SeekStart)
			if err != nil {
				return nil, err
			}
			return loadLegacyNoTar(f)
		default:
			return nil, err
		}
		// TODO: ...
		panic("legacy load from tar not implemented")
	}
}

func loadLegacyNoTar(f *os.File) (interface{}, error) {
	if err := readAndCheckMagicNumber(f); err != nil {
		return nil, err
	}
	if err := readAndChecProtocolVersion(f); err != nil {
		return nil, err
	}
	if _, err := unpickle(f); err != nil { // sys info
		return nil, err
	}

	deserializedObjects := make(map[string]StorageInterface)

	u := gopickle.NewUnpickler(f)
	u.FindClass = pickleFindClass
	u.PersistentLoad = func(savedId interface{}) (interface{}, error) {
		tuple, tupleOk := savedId.(*types.Tuple)
		if !tupleOk || tuple.Len() == 0 {
			return nil, fmt.Errorf("PersistentLoad: non-empty tuple espected")
		}
		typename, typenameOk := tuple.Get(0).(string)
		if !typenameOk {
			return nil, fmt.Errorf("PersistentLoad: cannot get typename")
		}

		switch typename {
		case "storage":
			if tuple.Len() != 6 {
				return nil, fmt.Errorf(
					"PersistentLoad: unexpected storage data length")
			}
			dataType, dataTypeOk := tuple.Get(1).(StorageClassInterface)
			rootKey, rootKeyOk := tuple.Get(2).(string)
			location, locationOk := tuple.Get(3).(string)
			size, sizeOk := tuple.Get(4).(int)
			viewMetadata := tuple.Get(5)
			if !dataTypeOk || !rootKeyOk || !locationOk || !sizeOk {
				return nil, fmt.Errorf("PersistentLoad: unexpected data types")
			}
			storage, storageExists := deserializedObjects[rootKey]
			if !storageExists {
				storage = dataType.New(size, location)
				deserializedObjects[rootKey] = storage
			}
			switch vm := viewMetadata.(type) {
			case nil:
				return storage, nil
			case []interface{}:
				if len(vm) != 3 {
					return nil, fmt.Errorf(
						"PersistentLoad: unexpected view metadata length")
				}
				panic("viewMetadata not implemented")
				// TODO: ...
				// view_key, offset, view_size = view_metadata
				// if view_key not in deserialized_objects:
				//     deserialized_objects[view_key] = storage[offset:offset + view_size]
				// return deserialized_objects[view_key]
			default:
				return nil, fmt.Errorf("PersistentLoad: unexpected view metadata type")
			}
		case "module":
			// TODO: ...
			// Ignore containers that don't have any sources saved
			// if all(data[1:]):
			//     _check_container_source(*data)
			// return data[0]
			panic("PersistentLoad module not implemented")
		default:
			return nil, fmt.Errorf("Unexpected saved ID type: %s", typename)
		}
	}
	result, err := u.Load()
	if err != nil {
		return nil, err
	}

	rawStorageKeys, err := unpickle(f)
	if err != nil {
		return nil, err
	}
	storageKeys, err := makeStorageKeys(rawStorageKeys)
	if err != nil {
		return nil, err
	}

	for _, key := range storageKeys {
		storageObj, ok := deserializedObjects[key]
		if !ok {
			return nil, fmt.Errorf("storage object not found for key '%s'", key)
		}
		err = storageObj.SetFromFile(f)
		if err != nil {
			return nil, err
		}
	}

	return result, nil
}

func makeStorageKeys(obj interface{}) ([]string, error) {
	list, ok := obj.(*types.List)
	if !ok {
		return nil, fmt.Errorf("invalid storage keys data")
	}
	keys := make([]string, len(*list))
	for i, rawKey := range *list {
		key, keyOk := rawKey.(string)
		if !keyOk {
			return nil, fmt.Errorf("invalid storage key")
		}
		keys[i] = key
	}
	return keys, nil
}

func readAndCheckMagicNumber(r io.Reader) error {
	obj, err := unpickle(r)
	if err != nil {
		return err
	}
	if n, ok := obj.(*big.Int); !ok || n.Text(16) != hexMagicNumber {
		return ErrInvalidMagicNumber
	}
	return nil
}

func readAndChecProtocolVersion(r io.Reader) error {
	obj, err := unpickle(r)
	if err != nil {
		return err
	}
	if n, ok := obj.(int); !ok || n != protocolVersion {
		return ErrInvalidProtocolVersion
	}
	return nil
}

func unpickle(r io.Reader) (interface{}, error) {
	u := gopickle.NewUnpickler(r)
	return u.Load()
}

func isZipFile(filename string) bool {
	r, err := zip.OpenReader(filename)
	if err != nil {
		return false
	}
	r.Close()
	return true
}

func pickleFindClass(module, name string) (interface{}, error) {
	switch module + "." + name {
	case "torch._utils._rebuild_tensor_v2":
		return &RebuildTensorV2{}, nil
	case "torch.FloatStorage":
		return &FloatStorageClass{}, nil
	case "torch.HalfStorage":
		return &HalfStorageClass{}, nil
	case "torch.DoubleStorage":
		return &DoubleStorageClass{}, nil
	case "torch.CharStorage":
		return &CharStorageClass{}, nil
	case "torch.ShortStorage":
		return &ShortStorageClass{}, nil
	case "torch.IntStorage":
		return &IntStorageClass{}, nil
	case "torch.LongStorage":
		return &LongStorageClass{}, nil
	case "torch.ByteStorage":
		return &ByteStorageClass{}, nil
	case "torch.BoolStorage":
		return &BoolStorageClass{}, nil
	default:
		return nil, fmt.Errorf("class no found: %s %s", module, name)
	}
}