# Wrapped link closers

CommonMark 4.7 and 6.6 allow one line ending in the whitespace around a link's
destination, its title and its reference label. Inside a container the bytes
after that line ending carry the marker the parser strips.

> See [the docs](/docs
> 'Documentation') here.

> See ![the diagram](/diagram.png
> 'A diagram') here.

>> See [the deeper docs](/deeper
>> "Deeper") here.

> See [the reference][
> label] here.

- See [an item's link](/item
  'An item') here.

[ label]: /label
