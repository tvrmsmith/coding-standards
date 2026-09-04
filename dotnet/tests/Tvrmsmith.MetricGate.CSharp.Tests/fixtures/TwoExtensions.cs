namespace Fixtures;

// Two C# 14 extension blocks declaring the same member name. Neither block contributes a name
// segment, so both accessors qualify to "Multi.get_IsLong" and the receiver parameter in the
// signature is the only thing that tells them apart. Line range is not enough: OneLine below writes
// both blocks on a single line, so name, line start and line end all agree. A collision here would
// abort the gate run on valid C#.
public static class Multi
{
    extension(string text)
    {
        public bool IsLong => text.Length > 10;
    }

    extension(int number)
    {
        public bool IsLong => number > 10;
    }
}

public static class OneLine
{
    extension(string t) { public bool Big() => true; } extension(int n) { public bool Big() => true; }
}
