namespace Fixtures;

public class Points
{
    public int All(int a, string? s)
    {
        var x = s ?? "d";
        var y = a > 0 ? 1 : 2;
        for (var i = 0; i < a; i++)
        {
            y += i;
        }

        do
        {
            y--;
        }
        while (y > 0);

        try
        {
            switch (a)
            {
                case 1:
                    return 1;
                case 2:
                    return 2;
                default:
                    return x.Length + y;
            }
        }
        catch (Exception e) when (e is InvalidOperationException)
        {
            return -1;
        }
    }
}
