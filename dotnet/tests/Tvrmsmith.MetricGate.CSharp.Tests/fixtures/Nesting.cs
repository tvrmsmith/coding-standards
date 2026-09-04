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

    public int ThreeDeep(int n)
    {
        int Outer(int a)
        {
            int Middle(int b)
            {
                int Innermost(int c)
                {
                    if (c > 0)
                    {
                        return c;
                    }

                    return 0;
                }

                if (b > 0)
                {
                    return Innermost(b);
                }

                return 0;
            }

            if (a > 0)
            {
                return Middle(a);
            }

            return 0;
        }

        return Outer(n);
    }

    private int _value;

    private System.Action _initializerLocal = () =>
    {
        void Detached(int n)
        {
            if (n > 0)
            {
                System.Console.WriteLine(n);
            }
        }

        Detached(1);
    };
}
