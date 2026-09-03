namespace Fixtures;

public class Plain
{
    public int Simple(int a)
    {
        return a + 1;
    }

    public int Branchy(int a, int b)
    {
        if (a > 0 && b > 0)
        {
            return 1;
        }

        while (a < b)
        {
            a++;
        }

        return a;
    }
}
