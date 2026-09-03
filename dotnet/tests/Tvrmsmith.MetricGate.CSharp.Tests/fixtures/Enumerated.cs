namespace Fixtures;

// One method per construct ComplexityWalker's contract enumerates, each holding that
// construct and nothing else, so a construct that stops scoring fails on its own name rather
// than moving an aggregate somebody has to decompose. The three at the bottom are the
// contract's stated non-points.
public class Enumerated
{
    public int If(int a)
    {
        if (a > 0)
        {
            return 1;
        }

        return 0;
    }

    public int While(int a)
    {
        while (a > 0)
        {
            a--;
        }

        return a;
    }

    public int Do(int a)
    {
        do
        {
            a--;
        }
        while (a > 0);

        return a;
    }

    public int For(int a)
    {
        for (var i = 0; i < a; i++)
        {
            a += i;
        }

        return a;
    }

    public int Foreach(int[] xs)
    {
        var total = 0;
        foreach (var x in xs)
        {
            total += x;
        }

        return total;
    }

    public int CaseLabel(int a)
    {
        switch (a)
        {
            case 1:
                return 1;
        }

        return 0;
    }

    public int CasePatternLabel(object o)
    {
        switch (o)
        {
            case int n:
                return n;
        }

        return 0;
    }

    public int CaseGuard(int a)
    {
        switch (a)
        {
            case int n when n > 0:
                return 1;
        }

        return 0;
    }

    public int SwitchExpressionArm(int a) => a switch
    {
        _ => 0,
    };

    public int Catch()
    {
        try
        {
            return 1;
        }
        catch (InvalidOperationException)
        {
            return 0;
        }
    }

    public int CatchFilter()
    {
        try
        {
            return 1;
        }
        catch (Exception e) when (e is InvalidOperationException)
        {
            return 0;
        }
    }

    public bool AndPattern(object o) => o is int and > 0;

    public bool OrPattern(object o) => o is int or string;

    public bool AndAlso(int a, int b) => a > 0 && b > 0;

    public bool OrElse(int a, int b) => a > 0 || b > 0;

    public int Conditional(int a) => a > 0 ? 1 : 0;

    public string Coalesce(string? s) => s ?? "d";

    public int DefaultLabel(int a)
    {
        switch (a)
        {
            default:
                return 0;
        }
    }

    public bool NotPattern(object? o) => o is not null;

    public string CoalesceAssign(string? s)
    {
        s ??= "d";
        return s;
    }

    public int Folded(int a)
    {
        int Inner(int x)
        {
            if (x > 0)
            {
                return 1;
            }

            return 0;
        }

        return Inner(a);
    }
}
