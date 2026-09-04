namespace Fixtures;

// A record qualifies its members the way a class does, and a record struct and a positional record
// with a body do too, so the qualification is pinned rather than left to fall out of
// RecordDeclarationSyntax happening to derive from TypeDeclarationSyntax. The positional parameter
// list is a primary constructor and still gets no span of its own.
public record Line(int Length)
{
    public bool IsLong => Length > 10;

    public int Doubled()
    {
        if (Length > 0)
        {
            return Length * 2;
        }

        return 0;
    }
}

public record struct Point
{
    public int X { get; set; }

    public int Away() => X;
}
