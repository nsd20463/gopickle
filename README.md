# GoPickle2JSON

GoPickle is a Go library for loading Python's data serialized with `pickle`
and converting it to JSON. This code is derived from the nlpodyssey/gopickle
repo (or more precisely, the nsd20463/gopickle fork of the nlpodyssey repo)
with the pytorch parts removed, and the output changed from Go objects
representing python datatypes to JSON text.

Some limitations have to apply when JSON is the output:
  * a python OrderDict becomes a JSON dictionary, so there is no room in the
    JSON for extra attributes outside of the dictionary entries, neither
    from self.__dict__ nor from self.setattr().
  * python can have internal references (ie. list element is the list itself)
    but JSON cannot.
  * no GenericObject is used for unknown picked object classes. If your pickled
    data contains classes outside of the supported classes, you need to handle
    then with a custom Unpickler.FindClass function, and implement the class's
    PyNew() and the instance's JSON() methods.

And some assumptions to improve performance:
  * pickled dictionaries and sets don't contain duplicate keys. We aren't
    going to spent space and time double checking something python already
    should have checked.

And, as with unpickling in python, don't unpickle data from strangers.

----------------------------------------------------------------------------

# README from parent gopickle repo:

The `pickle` sub-package provides the core functionality for loading data
serialized with Python `pickle` module, from a file, string, or byte sequence.
All _pickle_ protocols from 0 to 5 are supported.


## How it works

### Pickle

Unlike more traditional data serialization formats, (such as JSON or YAML),
a "pickle" is a _program_ for a so-called _unpickling machine_, also known
as _virtual pickle machine_, or _PM_ for short. A program consists in a
sequence of opcodes which instructs the virtual machine about how to build
arbitrarily complex Python objects. You can learn more  from Python
`pickletools` [module documentation](https://github.com/python/cpython/blob/3.8/Lib/pickletools.py).

Python PM implementation is straightforward, since it can take advantage
of the whole environment provided by a running Python interpreter. For this
Go implementation we want to keep things simple, for example avoiding
dependencies or foreign bindings, yet we want to provide flexibility, and a way
for any user to extend basic functionalities of the library.

This Go unpickling machine implementation makes use of a set of types defined
in `types`.
This sub-package contains Go types representing classes, instances and common
interfaces for some of the most commonly used builtin non-scalar types in 
Python.
We chose to provide only minimal functionalities for each type, for the sole 
purpose of making them easy to be handled by the machine.

Since Python's _pickle_ can dump and load _any_ object, the aforementioned types
are clearly not always sufficient. You can easily handle the loading of any 
missing class by explicitly providing a `FindClass` callback to an `Unpickler`
object. The implementation of your custom classes can be as simple or as
sophisticated as you need. If a certain class is required but is not found,
by default a `GenericClass` is used.
In some circumstances, this is enough to fully load a _pickle_ program, but
on other occasions the pickle program might require a certain class with
specific traits: in this case, the `GenericClass` is not enough and an error
is returned. You should be able to fix this situation providing
a custom class implementation, that jas to reflect the same basic behaviour
you can observe in the original Python implementation.

A similar approach is adopted for other peculiar aspects, such as persistent
objects loading, extensions handling, and a couple of protocol-5 opcodes:
whenever necessary, you can implement custom behaviours providing one or more
callback functions.

Once resolved, all representation of classes and objects are casted to
`interface{}` type; then the machine looks for specific types or
interfaces to be implemented on an object only where strictly necessary. 

The virtual machine closely follows the original implementation
from Python 3.8 - see the [`Unpickler` class](https://github.com/python/cpython/blob/3.8/Lib/pickle.py#L1134). 


## License

GoPickle is licensed under a BSD-style license.
See [LICENSE](https://github.com/mistsys/gopickle2json/blob/master/LICENSE) for
the full license text.
