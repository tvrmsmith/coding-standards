namespace Fixtures;

// One method per construct docs/csharp-decision-points.md enumerates, each holding that construct
// and nothing else, so a construct that stops scoring fails on its own name rather than moving an
// aggregate somebody has to decompose. DefaultLabel through BitwiseOrOperator are the constructs
// that document says score nothing; four of them, DefaultLabel, ConditionalAccess,
// BitwiseAndOperator and BitwiseOrOperator, are deltas where Roslyn does score.
// Folded at the end pins a nested local function's points to the local function alone.
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

    public int DeconstructingForeach((int, int)[] pairs)
    {
        var total = 0;
        foreach (var (a, b) in pairs)
        {
            total += a + b;
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

    public int Goto(int a)
    {
        goto done;

    done:
        return a;
    }

    public int? ConditionalAccess(string? s) => s?.Length;

    public bool BitwiseAndOperator(bool a, bool b) => a & b;

    public bool BitwiseOrOperator(bool a, bool b) => a | b;

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
