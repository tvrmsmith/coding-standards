namespace Fixtures;

public class Constructs
{
    public int Each(int[] xs)
    {
        var total = 0;
        foreach (var x in xs)
        {
            total += x;
        }

        return total;
    }

    public bool Either(int a, int b) => a > 0 || b > 0;

    public int Constant(object o)
    {
        switch (o)
        {
            case 1:
                return 1;
            default:
                return 0;
        }
    }

    public int Typed(object o)
    {
        switch (o)
        {
            case int n:
                return n;
            default:
                return 0;
        }
    }

    public int Guarded(int x)
    {
        switch (x)
        {
            case 1 when x > 0:
                return 1;
            default:
                return 0;
        }
    }
}
