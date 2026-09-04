namespace Fixtures;

// Two C# 14 extension blocks declaring the same member name. Neither block contributes a name
// segment, so both accessors qualify to "Multi.get_IsLong" and only the line range tells them
// apart. A collision here would abort the gate run on valid C#.
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
