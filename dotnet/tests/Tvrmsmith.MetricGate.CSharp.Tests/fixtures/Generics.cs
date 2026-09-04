using System.Collections.Generic;

namespace Fixtures;

// Type parameters are part of a declaration's name, so both a generic type and a generic method put
// a comma in the qualified name the document prints.
public class Generics<TKey, TValue>
{
    public Dictionary<TKey, TValue> Map<TA, TB>(TA a, TB b) => new();

    public int Single<T>(T value) => 1;

    public int Plain(int value) => value;
}
