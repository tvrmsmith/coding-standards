using System.Linq;

namespace Fixtures;

// A lambda's branches belong to the method holding it; a local function's belong to the local
// function, which carries a span of its own.
public class Nesting
{
    public int WithLambda(int[] xs)
    {
        var evens = xs.Where(x => x % 2 == 0 && x > 0);
        return evens.Count();
    }

    public int WithLocal(int[] xs)
    {
        int Running(int x)
        {
            if (x > 0)
            {
                return x;
            }

            return 0;
        }

        var total = 0;
        foreach (var x in xs)
        {
            total += Running(x);
        }

        return total;
    }

    public int LocalInsideLambda(int[] xs)
    {
        return xs.Select(x =>
        {
            int Twice(int n) => n > 0 ? n * 2 : 0;
            return Twice(x);
        }).Count();
    }

    public int Accessed
    {
        get
        {
            int Sign(int n)
            {
                if (n > 0)
                {
                    return 1;
                }

                return -1;
            }

            return Sign(_value);
        }
    }

    private int _value;
}
