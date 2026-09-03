namespace Fixtures;

public class Patterns
{
    public int Arms(int x) => x switch
    {
        1 => 1,
        2 => 2,
        _ => 0,
    };

    public int GuardedArm(int x) => x switch
    {
        var n when n > 0 => 1,
        _ => 0,
    };

    public bool OrPattern(object o) => o is int or string;

    public bool AndPattern(object o) => o is int and > 0;

    public bool OrOperator(object o) => o is int || o is string;

    public bool NotPattern(object o) => o is not null;
}
