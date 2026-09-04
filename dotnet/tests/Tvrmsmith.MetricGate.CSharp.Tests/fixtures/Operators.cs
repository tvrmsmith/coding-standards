namespace Fixtures;

// Every operator token C# lets a type overload, one declaration each, so a typo or a swapped pair in
// the metadata-name table fails under the name it got wrong rather than silently detaching a member
// from its coverage row. The names here are the ones the real compiler emits, read back off a
// compiled probe. The trailing block is C# 14's instance compound assignment operators, which name a
// different member from the classic static forms above them.
public class Ops
{
    public static Ops operator *(Ops a, Ops b) => a;

    public static Ops operator /(Ops a, Ops b) => a;

    public static Ops operator %(Ops a, Ops b) => a;

    public static Ops operator &(Ops a, Ops b) => a;

    public static Ops operator |(Ops a, Ops b) => a;

    public static Ops operator ^(Ops a, Ops b) => a;

    public static Ops operator <<(Ops a, int n) => a;

    public static Ops operator >>(Ops a, int n) => a;

    public static Ops operator >>>(Ops a, int n) => a;

    public static bool operator ==(Ops a, Ops b) => true;

    public static bool operator !=(Ops a, Ops b) => false;

    public static bool operator <(Ops a, Ops b) => true;

    public static bool operator >(Ops a, Ops b) => false;

    public static bool operator <=(Ops a, Ops b) => true;

    public static bool operator >=(Ops a, Ops b) => false;

    public static bool operator !(Ops a) => true;

    public static Ops operator ~(Ops a) => a;

    public static Ops operator ++(Ops a) => a;

    public static Ops operator --(Ops a) => a;

    public static bool operator true(Ops a) => true;

    public static bool operator false(Ops a) => false;
}

public class Compounds
{
    public void operator +=(Compounds other) { }

    public void operator -=(Compounds other) { }

    public void operator *=(Compounds other) { }

    public void operator /=(Compounds other) { }

    public void operator %=(Compounds other) { }

    public void operator &=(Compounds other) { }

    public void operator |=(Compounds other) { }

    public void operator ^=(Compounds other) { }

    public void operator <<=(int n) { }

    public void operator >>=(int n) { }

    public void operator >>>=(int n) { }

    public void operator ++() { }

    public void operator --() { }
}
